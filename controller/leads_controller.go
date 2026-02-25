package controller

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"konvoq-backend/utils"
)

func (c *Controller) ListLeads(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,name,email,phone,status,created_at FROM leads WHERE user_id=$1 ORDER BY created_at DESC`, claims.UserID)
	if err != nil {
		c.logRequestError(r, "list leads query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id string
		var name, email, phone, status sql.NullString
		var created time.Time
		if err := rows.Scan(&id, &name, &email, &phone, &status, &created); err != nil {
			c.logRequestWarn(r, "list leads row scan failed", err, "user_id", claims.UserID)
			continue
		}
		items = append(items, map[string]interface{}{
			"id": id, "name": utils.NullString(name), "email": utils.NullString(email),
			"phone": utils.NullString(phone), "status": utils.NullString(status), "createdAt": created,
		})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "leads": items})
}

func (c *Controller) GetLead(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	id := chi.URLParam(r, "id")
	var name, email, phone, status sql.NullString
	var created time.Time
	err := c.db.QueryRow(`SELECT name,email,phone,status,created_at FROM leads WHERE id=$1 AND user_id=$2`, id, claims.UserID).Scan(&name, &email, &phone, &status, &created)
	if err != nil {
		if err != sql.ErrNoRows {
			c.logRequestError(r, "get lead query failed", err, "user_id", claims.UserID, "lead_id", id)
		}
		utils.JSONErr(w, http.StatusNotFound, "lead not found")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "lead": map[string]interface{}{
		"id": id, "name": utils.NullString(name), "email": utils.NullString(email),
		"phone": utils.NullString(phone), "status": utils.NullString(status), "createdAt": created,
	}})
}

func (c *Controller) UpdateLeadStatus(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	id := chi.URLParam(r, "id")
	var body struct {
		Status string `json:"status"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Status) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "status is required")
		return
	}
	_, err := c.db.Exec(`UPDATE leads SET status=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND user_id=$2`, id, claims.UserID, body.Status)
	if err != nil {
		c.logRequestError(r, "update lead status failed", err, "user_id", claims.UserID, "lead_id", id, "status", body.Status)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.GetLead(w, r, claims, UserRecord{})
}

func (c *Controller) DeleteLead(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	id := chi.URLParam(r, "id")
	if _, err := c.db.Exec(`DELETE FROM leads WHERE id=$1 AND user_id=$2`, id, claims.UserID); err != nil {
		c.logRequestError(r, "delete lead failed", err, "user_id", claims.UserID, "lead_id", id)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) GetLeadWebhook(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if user.PlanType != "enterprise" {
		utils.JSONErr(w, http.StatusForbidden, "This feature is available only for enterprise plan")
		return
	}
	var id, webhookURL string
	var active bool
	var created, updated time.Time
	err := c.db.QueryRow(`SELECT id,webhook_url,is_active,created_at,updated_at FROM lead_webhook_configs WHERE user_id=$1`, claims.UserID).Scan(&id, &webhookURL, &active, &created, &updated)
	if err == sql.ErrNoRows {
		utils.JSONOK(w, map[string]interface{}{"success": true, "config": nil})
		return
	}
	if err != nil {
		c.logRequestError(r, "get lead webhook config failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "config": map[string]interface{}{
		"id": id, "webhookUrl": webhookURL, "isActive": active, "createdAt": created, "updatedAt": updated,
	}})
}

func (c *Controller) UpsertLeadWebhook(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if user.PlanType != "enterprise" {
		utils.JSONErr(w, http.StatusForbidden, "This feature is available only for enterprise plan")
		return
	}
	var body struct {
		WebhookURL string `json:"webhookUrl"`
		IsActive   *bool  `json:"isActive"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.WebhookURL) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "webhookUrl is required")
		return
	}
	active := true
	if body.IsActive != nil {
		active = *body.IsActive
	}
	secret := utils.RandomID("whsec")
	_, err := c.db.Exec(`INSERT INTO lead_webhook_configs (user_id,webhook_url,signing_secret,is_active) VALUES ($1,$2,$3,$4)
		ON CONFLICT (user_id) DO UPDATE SET webhook_url=EXCLUDED.webhook_url,is_active=EXCLUDED.is_active,updated_at=CURRENT_TIMESTAMP`,
		claims.UserID, body.WebhookURL, secret, active)
	if err != nil {
		c.logRequestError(r, "upsert lead webhook config failed", err, "user_id", claims.UserID, "webhook_url", body.WebhookURL)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.GetLeadWebhook(w, r, claims, user)
}

func (c *Controller) LeadWebhookTest(w http.ResponseWriter, _ *http.Request, claims TokenClaims, user UserRecord) {
	if user.PlanType != "enterprise" {
		utils.JSONErr(w, http.StatusForbidden, "This feature is available only for enterprise plan")
		return
	}
	c.queueWebhookEvent(claims.UserID, "", "lead.test", map[string]interface{}{
		"eventType": "lead.test",
		"message":   "This is a test webhook event from Witzo",
		"emittedAt": time.Now().UTC().Format(time.RFC3339),
	})
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) ListWebhookEvents(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if user.PlanType != "enterprise" {
		utils.JSONErr(w, http.StatusForbidden, "This feature is available only for enterprise plan")
		return
	}
	rows, err := c.db.Query(`SELECT id,lead_id,event_type,status,attempts,max_attempts,next_attempt_at,last_error,response_status,last_attempt_at,delivered_at,created_at FROM lead_webhook_events WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`,
		claims.UserID)
	if err != nil {
		c.logRequestError(r, "list webhook events query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
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
		if err := rows.Scan(&id, &leadID, &eventType, &status, &attempts, &maxAttempts, &nextAt, &lastErr, &resp, &lastAt, &deliveredAt, &created); err != nil {
			c.logRequestWarn(r, "list webhook events row scan failed", err, "user_id", claims.UserID)
			continue
		}
		items = append(items, map[string]interface{}{
			"id": id, "leadId": utils.NullString(leadID), "eventType": eventType, "status": status,
			"attempts": attempts, "maxAttempts": maxAttempts, "nextAttemptAt": nextAt,
			"lastError": utils.NullString(lastErr), "responseStatus": utils.NullableInt64(resp),
			"lastAttemptAt": utils.NullTime(lastAt), "deliveredAt": utils.NullTime(deliveredAt), "createdAt": created,
		})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "events": items})
}

func (c *Controller) RetryWebhookEvent(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if user.PlanType != "enterprise" {
		utils.JSONErr(w, http.StatusForbidden, "This feature is available only for enterprise plan")
		return
	}
	eid := chi.URLParam(r, "id")
	if _, err := c.db.Exec(`UPDATE lead_webhook_events SET status='pending',attempts=0,next_attempt_at=CURRENT_TIMESTAMP,last_error=NULL,response_status=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND user_id=$2`,
		eid, claims.UserID); err != nil {
		c.logRequestError(r, "retry webhook event update failed", err, "user_id", claims.UserID, "event_id", eid)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}
