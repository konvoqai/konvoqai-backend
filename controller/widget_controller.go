package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"konvoq-backend/utils"
)

func (c *Controller) CreateWidget(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body struct {
		Name        string                 `json:"name"`
		Theme       string                 `json:"theme"`
		Primary     string                 `json:"primaryColor"`
		WelcomeText string                 `json:"welcomeText"`
		Settings    map[string]interface{} `json:"settings"`
	}
	_ = utils.DecodeJSON(r, &body)
	cfg := map[string]interface{}{"theme": body.Theme, "primaryColor": body.Primary, "welcomeText": body.WelcomeText}
	for k, v := range body.Settings {
		cfg[k] = v
	}
	cfgJSON, _ := json.Marshal(cfg)
	widgetKey := utils.RandomID("wk")
	var id int64
	err := c.db.QueryRow(`INSERT INTO widget_keys (user_id,widget_key,widget_name,widget_config,is_active) VALUES ($1,$2,$3,$4::jsonb,TRUE)
		ON CONFLICT (user_id) DO UPDATE SET widget_key=EXCLUDED.widget_key,widget_name=EXCLUDED.widget_name,widget_config=EXCLUDED.widget_config,is_active=TRUE,updated_at=CURRENT_TIMESTAMP
		RETURNING id`,
		claims.UserID, widgetKey, coalesce(body.Name, "My Chat Widget"), string(cfgJSON)).Scan(&id)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	_ = c.redis.Del(ctx, "widget:"+widgetKey).Err()
	utils.JSONOK(w, map[string]interface{}{"success": true, "widget": map[string]interface{}{
		"id": id, "userId": claims.UserID, "widgetKey": widgetKey,
		"name": coalesce(body.Name, "My Chat Widget"), "settings": cfg,
	}})
}

func (c *Controller) GetWidget(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	var id int64
	var key, name string
	var active bool
	var cfgRaw []byte
	var created, updated time.Time
	err := c.db.QueryRow(`SELECT id,widget_key,widget_name,is_active,widget_config,created_at,updated_at FROM widget_keys WHERE user_id=$1`,
		claims.UserID).Scan(&id, &key, &name, &active, &cfgRaw, &created, &updated)
	if err != nil {
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	cfg := map[string]interface{}{}
	_ = json.Unmarshal(cfgRaw, &cfg)
	utils.JSONOK(w, map[string]interface{}{"success": true, "widget": map[string]interface{}{
		"id": id, "userId": claims.UserID, "widgetKey": key, "name": name,
		"isActive": active, "settings": cfg, "createdAt": created, "updatedAt": updated,
	}})
}

func (c *Controller) UpdateWidget(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body map[string]interface{}
	if err := utils.DecodeJSON(r, &body); err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid payload")
		return
	}
	name, _ := body["name"].(string)
	cfgJSON, _ := json.Marshal(body)
	_, err := c.db.Exec(`UPDATE widget_keys SET widget_name=COALESCE($2,widget_name),widget_config=COALESCE($3::jsonb,widget_config),updated_at=CURRENT_TIMESTAMP WHERE user_id=$1`,
		claims.UserID, utils.Nullable(name), string(cfgJSON))
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.GetWidget(w, r, claims, UserRecord{})
}

func (c *Controller) RegenerateWidget(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	newKey := utils.RandomID("wk")
	_, err := c.db.Exec(`UPDATE widget_keys SET widget_key=$2,updated_at=CURRENT_TIMESTAMP WHERE user_id=$1`, claims.UserID, newKey)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	_ = c.redis.Del(ctx, "widget:"+newKey).Err()
	c.GetWidget(w, r, claims, UserRecord{})
}

func (c *Controller) DeleteWidget(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	_, _ = c.db.Exec(`DELETE FROM widget_keys WHERE user_id=$1`, claims.UserID)
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) WidgetAnalytics(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	limit := 100
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	rows, err := c.db.Query(`SELECT wa.event_type,wa.event_data,wa.created_at FROM widget_analytics wa JOIN widget_keys wk ON wk.id=wa.widget_key_id WHERE wk.user_id=$1 ORDER BY wa.created_at DESC LIMIT $2`,
		claims.UserID, limit)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var et string
		var data []byte
		var created time.Time
		_ = rows.Scan(&et, &data, &created)
		m := map[string]interface{}{}
		_ = json.Unmarshal(data, &m)
		items = append(items, map[string]interface{}{"eventType": et, "eventData": m, "createdAt": created})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "analytics": items})
}
