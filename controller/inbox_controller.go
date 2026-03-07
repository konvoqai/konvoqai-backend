package controller

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"konvoq-backend/utils"
)

func (c *Controller) ListHandoffs(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	limits := limitsForPlan(user.PlanType)
	if !limits.HasHybrid {
		utils.JSONOK(w, map[string]interface{}{"success": true, "hasHybrid": false, "handoffs": []interface{}{}})
		return
	}

	status := r.URL.Query().Get("status")
	query := `SELECT id, session_id, status, claimed_by, visitor_name, visitor_email, trigger_reason, created_at, updated_at
		FROM handoff_requests WHERE user_id=$1`
	args := []interface{}{claims.UserID}
	if status != "" {
		query += ` AND status=$2`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC LIMIT 100`

	rows, err := c.db.Query(query, args...)
	if err != nil {
		c.logRequestError(r, "list handoffs query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	items := []map[string]interface{}{}
	for rows.Next() {
		var id, sessionID, handoffStatus string
		var claimedBy, visitorName, visitorEmail, triggerReason *string
		var created, updated time.Time
		if err := rows.Scan(&id, &sessionID, &handoffStatus, &claimedBy, &visitorName, &visitorEmail, &triggerReason, &created, &updated); err != nil {
			continue
		}
		item := map[string]interface{}{
			"id": id, "sessionId": sessionID, "status": handoffStatus,
			"createdAt": created, "updatedAt": updated,
		}
		if claimedBy != nil {
			item["claimedBy"] = *claimedBy
		}
		if visitorName != nil {
			item["visitorName"] = *visitorName
		}
		if visitorEmail != nil {
			item["visitorEmail"] = *visitorEmail
		}
		if triggerReason != nil {
			item["triggerReason"] = *triggerReason
		}
		items = append(items, item)
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "hasHybrid": true, "handoffs": items})
}

func (c *Controller) ClaimHandoff(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	id := chi.URLParam(r, "id")
	_, err := c.db.Exec(`
		UPDATE handoff_requests SET status='claimed', claimed_by=$3, updated_at=CURRENT_TIMESTAMP
		WHERE id=$1 AND user_id=$2 AND status='pending'`,
		id, claims.UserID, claims.Email)
	if err != nil {
		c.logRequestError(r, "claim handoff failed", err, "user_id", claims.UserID, "handoff_id", id)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) ResolveHandoff(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	id := chi.URLParam(r, "id")
	_, err := c.db.Exec(`
		UPDATE handoff_requests SET status='resolved', updated_at=CURRENT_TIMESTAMP
		WHERE id=$1 AND user_id=$2`,
		id, claims.UserID)
	if err != nil {
		c.logRequestError(r, "resolve handoff failed", err, "user_id", claims.UserID, "handoff_id", id)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) GetHandoffMessages(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	id := chi.URLParam(r, "id")
	// verify ownership
	var count int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM handoff_requests WHERE id=$1 AND user_id=$2`, id, claims.UserID).Scan(&count); err != nil || count == 0 {
		utils.JSONErr(w, http.StatusNotFound, "handoff not found")
		return
	}

	rows, err := c.db.Query(`SELECT id, sender_type, sender_email, content, created_at FROM handoff_messages WHERE handoff_id=$1 ORDER BY created_at ASC`, id)
	if err != nil {
		c.logRequestError(r, "get handoff messages query failed", err, "handoff_id", id)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	messages := []map[string]interface{}{}
	for rows.Next() {
		var msgID, senderType string
		var senderEmail *string
		var content string
		var created time.Time
		if err := rows.Scan(&msgID, &senderType, &senderEmail, &content, &created); err != nil {
			continue
		}
		msg := map[string]interface{}{"id": msgID, "senderType": senderType, "content": content, "createdAt": created}
		if senderEmail != nil {
			msg["senderEmail"] = *senderEmail
		}
		messages = append(messages, msg)
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "messages": messages})
}

func (c *Controller) SendHandoffMessage(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	id := chi.URLParam(r, "id")
	// verify ownership
	var count int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM handoff_requests WHERE id=$1 AND user_id=$2`, id, claims.UserID).Scan(&count); err != nil || count == 0 {
		utils.JSONErr(w, http.StatusNotFound, "handoff not found")
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Content) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "content is required")
		return
	}

	var msgID string
	err := c.db.QueryRow(`
		INSERT INTO handoff_messages (handoff_id, sender_type, sender_email, content)
		VALUES ($1, 'agent', $2, $3) RETURNING id`,
		id, claims.Email, strings.TrimSpace(body.Content)).Scan(&msgID)
	if err != nil {
		c.logRequestError(r, "send handoff message failed", err, "handoff_id", id)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{
		"success": true,
		"message": map[string]interface{}{
			"id": msgID, "senderType": "agent", "senderEmail": claims.Email,
			"content": strings.TrimSpace(body.Content), "createdAt": time.Now(),
		},
	})
}

// RequestHandoff — can be called by widget visitor to request human agent
func (c *Controller) RequestHandoff(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	var body struct {
		SessionID     string `json:"sessionId"`
		VisitorName   string `json:"visitorName"`
		VisitorEmail  string `json:"visitorEmail"`
		TriggerReason string `json:"triggerReason"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.SessionID) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "sessionId is required")
		return
	}

	var id string
	err := c.db.QueryRow(`
		INSERT INTO handoff_requests (user_id, session_id, visitor_name, visitor_email, trigger_reason)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		claims.UserID,
		strings.TrimSpace(body.SessionID),
		utils.Nullable(strings.TrimSpace(body.VisitorName)),
		utils.Nullable(strings.TrimSpace(body.VisitorEmail)),
		utils.Nullable(strings.TrimSpace(body.TriggerReason))).Scan(&id)
	if err != nil {
		c.logRequestError(r, "request handoff insert failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "handoffId": id})
}
