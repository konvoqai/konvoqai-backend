package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"konvoq-backend/utils"
)

type BrandingSettings struct {
	HideBranding    bool   `json:"hideBranding"`
	DashboardLogo   string `json:"dashboardLogo"`
	DashboardName   string `json:"dashboardName"`
	DashboardFavicon string `json:"dashboardFavicon"`
}

func (c *Controller) GetBranding(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	var cfgRaw []byte
	err := c.db.QueryRow(`SELECT widget_config FROM widget_keys WHERE user_id=$1`, claims.UserID).Scan(&cfgRaw)
	if err != nil {
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	cfg := map[string]interface{}{}
	_ = json.Unmarshal(cfgRaw, &cfg)

	branding := BrandingSettings{}
	if raw, ok := cfg["branding"]; ok {
		if b, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(b, &branding)
		}
	}

	limits := limitsForPlan(user.PlanType)
	utils.JSONOK(w, map[string]interface{}{
		"success":       true,
		"branding":      branding,
		"hideBranding":  limits.HideBranding,
	})
}

func (c *Controller) UpdateBranding(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	limits := limitsForPlan(user.PlanType)
	if !limits.HideBranding {
		utils.JSONErr(w, http.StatusPaymentRequired, "custom branding requires PRO plan or above")
		return
	}

	var body BrandingSettings
	if err := utils.DecodeJSON(r, &body); err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid payload")
		return
	}
	body.DashboardLogo = strings.TrimSpace(body.DashboardLogo)
	body.DashboardName = strings.TrimSpace(body.DashboardName)
	body.DashboardFavicon = strings.TrimSpace(body.DashboardFavicon)

	patch := map[string]interface{}{"branding": body}
	patchJSON, _ := json.Marshal(patch)

	var widgetKey string
	err := c.db.QueryRow(`UPDATE widget_keys SET widget_config = widget_config || $2::jsonb, updated_at=CURRENT_TIMESTAMP WHERE user_id=$1 RETURNING widget_key`,
		claims.UserID, string(patchJSON)).Scan(&widgetKey)
	if err != nil {
		c.logRequestError(r, "update branding failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	_ = c.redis.Del(ctx, "widget:"+widgetKey).Err()

	utils.JSONOK(w, map[string]interface{}{"success": true, "branding": body})
}
