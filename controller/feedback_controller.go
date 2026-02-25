package controller

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"konvoq-backend/utils"
)

func (c *Controller) ListFeedback(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,type,title,message,page_path,created_at FROM feedback_suggestions WHERE user_id=$1 ORDER BY created_at DESC LIMIT 200`, claims.UserID)
	if err != nil {
		c.logRequestError(r, "list feedback query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id, typ, msg string
		var title, page sql.NullString
		var created time.Time
		if err := rows.Scan(&id, &typ, &title, &msg, &page, &created); err != nil {
			c.logRequestWarn(r, "list feedback row scan failed", err, "user_id", claims.UserID)
			continue
		}
		items = append(items, map[string]interface{}{
			"id": id, "type": typ, "title": utils.NullString(title),
			"message": msg, "pagePath": utils.NullString(page), "createdAt": created,
		})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "feedback": items})
}

func (c *Controller) CreateFeedback(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body struct {
		Type    string `json:"type"`
		Title   string `json:"title"`
		Message string `json:"message"`
		Page    string `json:"page_path"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Message) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "message is required")
		return
	}
	t := body.Type
	if t != "feedback" && t != "suggestion" {
		t = "feedback"
	}
	var id string
	err := c.db.QueryRow(`INSERT INTO feedback_suggestions (user_id,type,title,message,page_path,user_agent) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		claims.UserID, t, utils.Nullable(body.Title), body.Message, utils.Nullable(body.Page), utils.Nullable(r.UserAgent())).Scan(&id)
	if err != nil {
		c.logRequestError(r, "create feedback insert failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "feedback": map[string]interface{}{"id": id, "type": t, "message": body.Message}})
}
