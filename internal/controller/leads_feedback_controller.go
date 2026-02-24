package controller

import (
	"database/sql"
	"net/http"
	"strings"
	"time"
)

func (c *Controller) ListLeads(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,name,email,phone,status,created_at FROM leads WHERE user_id=$1 ORDER BY created_at DESC`, claims.UserID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id string
		var name, email, phone, status sql.NullString
		var created time.Time
		_ = rows.Scan(&id, &name, &email, &phone, &status, &created)
		items = append(items, map[string]interface{}{"id": id, "name": nullString(name), "email": nullString(email), "phone": nullString(phone), "status": nullString(status), "createdAt": created})
	}
	jsonOK(w, map[string]interface{}{"success": true, "leads": items})
}

func (c *Controller) GetLead(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	id := r.PathValue("id")
	var name, email, phone, status sql.NullString
	var created time.Time
	err := c.db.QueryRow(`SELECT name,email,phone,status,created_at FROM leads WHERE id=$1 AND user_id=$2`, id, claims.UserID).Scan(&name, &email, &phone, &status, &created)
	if err != nil {
		jsonErr(w, http.StatusNotFound, "lead not found")
		return
	}
	jsonOK(w, map[string]interface{}{"success": true, "lead": map[string]interface{}{"id": id, "name": nullString(name), "email": nullString(email), "phone": nullString(phone), "status": nullString(status), "createdAt": created}})
}

func (c *Controller) UpdateLeadStatus(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	id := r.PathValue("id")
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Status) == "" {
		jsonErr(w, http.StatusBadRequest, "status is required")
		return
	}
	_, err := c.db.Exec(`UPDATE leads SET status=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND user_id=$2`, id, claims.UserID, body.Status)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.GetLead(w, r, claims, UserRecord{})
}

func (c *Controller) DeleteLead(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	id := r.PathValue("id")
	_, _ = c.db.Exec(`DELETE FROM leads WHERE id=$1 AND user_id=$2`, id, claims.UserID)
	jsonOK(w, map[string]interface{}{"success": true})
}
func (c *Controller) GetLeadWebhook(w http.ResponseWriter, _ *http.Request, claims TokenClaims, user UserRecord) {
	if user.PlanType != "enterprise" {
		jsonErr(w, http.StatusForbidden, "This feature is available only for enterprise plan")
		return
	}
	var id, url string
	var active bool
	var created, updated time.Time
	err := c.db.QueryRow(`SELECT id,webhook_url,is_active,created_at,updated_at FROM lead_webhook_configs WHERE user_id=$1`, claims.UserID).Scan(&id, &url, &active, &created, &updated)
	if err == sql.ErrNoRows {
		jsonOK(w, map[string]interface{}{"success": true, "config": nil})
		return
	}
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	jsonOK(w, map[string]interface{}{"success": true, "config": map[string]interface{}{"id": id, "webhookUrl": url, "isActive": active, "createdAt": created, "updatedAt": updated}})
}

func (c *Controller) UpsertLeadWebhook(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if user.PlanType != "enterprise" {
		jsonErr(w, http.StatusForbidden, "This feature is available only for enterprise plan")
		return
	}
	var body struct {
		WebhookURL string `json:"webhookUrl"`
		IsActive   *bool  `json:"isActive"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.WebhookURL) == "" {
		jsonErr(w, http.StatusBadRequest, "webhookUrl is required")
		return
	}
	active := true
	if body.IsActive != nil {
		active = *body.IsActive
	}
	secret := randomID("whsec")
	_, err := c.db.Exec(`INSERT INTO lead_webhook_configs (user_id,webhook_url,signing_secret,is_active) VALUES ($1,$2,$3,$4)
		ON CONFLICT (user_id) DO UPDATE SET webhook_url=EXCLUDED.webhook_url,is_active=EXCLUDED.is_active,updated_at=CURRENT_TIMESTAMP`, claims.UserID, body.WebhookURL, secret, active)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.GetLeadWebhook(w, r, claims, user)
}

func (c *Controller) LeadWebhookTest(w http.ResponseWriter, _ *http.Request, claims TokenClaims, user UserRecord) {
	if user.PlanType != "enterprise" {
		jsonErr(w, http.StatusForbidden, "This feature is available only for enterprise plan")
		return
	}
	c.queueWebhookEvent(claims.UserID, "", "lead.test", map[string]interface{}{
		"eventType": "lead.test",
		"message":   "This is a test webhook event from Witzo",
		"emittedAt": time.Now().UTC().Format(time.RFC3339),
	})
	jsonOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) ListWebhookEvents(w http.ResponseWriter, _ *http.Request, claims TokenClaims, user UserRecord) {
	if user.PlanType != "enterprise" {
		jsonErr(w, http.StatusForbidden, "This feature is available only for enterprise plan")
		return
	}
	rows, err := c.db.Query(`SELECT id,lead_id,event_type,status,attempts,max_attempts,next_attempt_at,last_error,response_status,last_attempt_at,delivered_at,created_at FROM lead_webhook_events WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, claims.UserID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id string
		var leadID sql.NullString
		var eventType, status string
		var attempts, maxAttempts int
		var nextAt time.Time
		var lastErr sql.NullString
		var resp sql.NullInt64
		var lastAt, deliveredAt sql.NullTime
		var created time.Time
		_ = rows.Scan(&id, &leadID, &eventType, &status, &attempts, &maxAttempts, &nextAt, &lastErr, &resp, &lastAt, &deliveredAt, &created)
		items = append(items, map[string]interface{}{"id": id, "leadId": nullString(leadID), "eventType": eventType, "status": status, "attempts": attempts, "maxAttempts": maxAttempts, "nextAttemptAt": nextAt, "lastError": nullString(lastErr), "responseStatus": nullableInt64(resp), "lastAttemptAt": nullTime(lastAt), "deliveredAt": nullTime(deliveredAt), "createdAt": created})
	}
	jsonOK(w, map[string]interface{}{"success": true, "events": items})
}

func nullableInt64(v sql.NullInt64) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func (c *Controller) RetryWebhookEvent(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if user.PlanType != "enterprise" {
		jsonErr(w, http.StatusForbidden, "This feature is available only for enterprise plan")
		return
	}
	eid := r.PathValue("eventId")
	_, _ = c.db.Exec(`UPDATE lead_webhook_events SET status='pending',attempts=0,next_attempt_at=CURRENT_TIMESTAMP,last_error=NULL,response_status=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND user_id=$2`, eid, claims.UserID)
	jsonOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) ListFeedback(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,type,title,message,page_path,created_at FROM feedback_suggestions WHERE user_id=$1 ORDER BY created_at DESC LIMIT 200`, claims.UserID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id, typ, msg string
		var title, page sql.NullString
		var created time.Time
		_ = rows.Scan(&id, &typ, &title, &msg, &page, &created)
		items = append(items, map[string]interface{}{"id": id, "type": typ, "title": nullString(title), "message": msg, "pagePath": nullString(page), "createdAt": created})
	}
	jsonOK(w, map[string]interface{}{"success": true, "feedback": items})
}

func (c *Controller) CreateFeedback(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body struct {
		Type    string `json:"type"`
		Title   string `json:"title"`
		Message string `json:"message"`
		Page    string `json:"page_path"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Message) == "" {
		jsonErr(w, http.StatusBadRequest, "message is required")
		return
	}
	t := body.Type
	if t != "feedback" && t != "suggestion" {
		t = "feedback"
	}
	var id string
	err := c.db.QueryRow(`INSERT INTO feedback_suggestions (user_id,type,title,message,page_path,user_agent) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, claims.UserID, t, nullable(body.Title), body.Message, nullable(body.Page), nullable(r.UserAgent())).Scan(&id)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	jsonOK(w, map[string]interface{}{"success": true, "feedback": map[string]interface{}{"id": id, "type": t, "message": body.Message}})
}
