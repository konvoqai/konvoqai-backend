package controller

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"konvoq-backend/utils"
)

func (c *Controller) ListProjects(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	chatbots, err := c.listUserChatbots(r, claims.UserID)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	projects := make([]map[string]interface{}, 0, len(chatbots))
	for _, chatbot := range chatbots {
		projects = append(projects, map[string]interface{}{
			"id":      chatbot["id"],
			"name":    chatbot["name"],
			"chatbot": chatbot,
		})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "projects": projects})
}

func (c *Controller) ListChatbots(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	chatbots, err := c.listUserChatbots(r, claims.UserID)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "chatbots": chatbots})
}

func (c *Controller) listUserChatbots(r *http.Request, userID string) ([]map[string]interface{}, error) {
	rows, err := c.db.Query(`SELECT id,widget_key,widget_name,is_active,widget_config,created_at,updated_at
		FROM widget_keys
		WHERE user_id=$1
		ORDER BY created_at DESC`, userID)
	if err != nil {
		c.logRequestError(r, "list chatbots query failed", err, "user_id", userID)
		return nil, err
	}
	defer rows.Close()

	items := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int64
		var key, name string
		var active bool
		var cfgRaw []byte
		var createdAt, updatedAt time.Time
		if scanErr := rows.Scan(&id, &key, &name, &active, &cfgRaw, &createdAt, &updatedAt); scanErr != nil {
			c.logRequestWarn(r, "list chatbots row scan failed", scanErr, "user_id", userID)
			continue
		}
		cfg := map[string]interface{}{}
		_ = json.Unmarshal(cfgRaw, &cfg)
		var sources, docs int
		if countErr := c.db.QueryRow(`SELECT COUNT(*) FROM scraper_sources WHERE user_id=$1`, userID).Scan(&sources); countErr != nil && countErr != sql.ErrNoRows {
			c.logRequestWarn(r, "list chatbots source count failed", countErr, "user_id", userID)
		}
		if countErr := c.db.QueryRow(`SELECT COUNT(*) FROM documents WHERE user_id=$1`, userID).Scan(&docs); countErr != nil && countErr != sql.ErrNoRows {
			c.logRequestWarn(r, "list chatbots document count failed", countErr, "user_id", userID)
		}
		items = append(items, map[string]interface{}{
			"id":            id,
			"name":          name,
			"widgetKey":     key,
			"isActive":      active,
			"isConfigured":  isWidgetConfigured(cfg),
			"settings":      cfg,
			"sourceCount":   sources,
			"documentCount": docs,
			"createdAt":     createdAt,
			"updatedAt":     updatedAt,
		})
	}
	return items, nil
}
