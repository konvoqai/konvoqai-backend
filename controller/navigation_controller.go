package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"konvoq-backend/utils"
)

type NavItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	URL   string `json:"url"`
	Icon  string `json:"icon"`
	Order int    `json:"order"`
}

func (c *Controller) GetNavigation(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	var cfgRaw []byte
	err := c.db.QueryRow(`SELECT widget_config FROM widget_keys WHERE user_id=$1`, claims.UserID).Scan(&cfgRaw)
	if err != nil {
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	cfg := map[string]interface{}{}
	_ = json.Unmarshal(cfgRaw, &cfg)

	navItems := []NavItem{}
	if raw, ok := cfg["navigation"]; ok {
		if b, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(b, &navItems)
		}
	}

	limits := limitsForPlan(user.PlanType)
	utils.JSONOK(w, map[string]interface{}{
		"success":       true,
		"navItems":      navItems,
		"hasNavigation": limits.HasNavigation,
	})
}

func (c *Controller) UpdateNavigation(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	limits := limitsForPlan(user.PlanType)
	if !limits.HasNavigation {
		utils.JSONErr(w, http.StatusPaymentRequired, "navigation builder requires PRO plan or above")
		return
	}

	var body struct {
		NavItems []NavItem `json:"navItems"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if len(body.NavItems) > 10 {
		utils.JSONErr(w, http.StatusBadRequest, "maximum 10 navigation items allowed")
		return
	}
	for i := range body.NavItems {
		body.NavItems[i].Label = strings.TrimSpace(body.NavItems[i].Label)
		body.NavItems[i].URL = strings.TrimSpace(body.NavItems[i].URL)
		body.NavItems[i].Icon = strings.TrimSpace(body.NavItems[i].Icon)
		body.NavItems[i].Order = i
	}

	patch := map[string]interface{}{"navigation": body.NavItems}
	patchJSON, _ := json.Marshal(patch)

	var widgetKey string
	err := c.db.QueryRow(`UPDATE widget_keys SET widget_config = widget_config || $2::jsonb, updated_at=CURRENT_TIMESTAMP WHERE user_id=$1 RETURNING widget_key`,
		claims.UserID, string(patchJSON)).Scan(&widgetKey)
	if err != nil {
		c.logRequestError(r, "update navigation failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	_ = c.redis.Del(ctx, "widget:"+widgetKey).Err()

	utils.JSONOK(w, map[string]interface{}{"success": true, "navItems": body.NavItems})
}
