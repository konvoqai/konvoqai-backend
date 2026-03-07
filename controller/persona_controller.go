package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"konvoq-backend/utils"
)

type PersonaSettings struct {
	Role         string `json:"role"`
	BotName      string `json:"botName"`
	Tone         string `json:"tone"`
	Instructions string `json:"instructions"`
}

func (c *Controller) GetPersona(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	var cfgRaw []byte
	err := c.db.QueryRow(`SELECT widget_config FROM widget_keys WHERE user_id=$1`, claims.UserID).Scan(&cfgRaw)
	if err != nil {
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	cfg := map[string]interface{}{}
	_ = json.Unmarshal(cfgRaw, &cfg)

	persona := PersonaSettings{Role: "default", BotName: "", Tone: "friendly", Instructions: ""}
	if raw, ok := cfg["persona"]; ok {
		if b, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(b, &persona)
		}
	}

	limits := limitsForPlan(user.PlanType)
	utils.JSONOK(w, map[string]interface{}{
		"success": true,
		"persona": persona,
		"availableRoles": limits.Roles,
		"hasPersona":     limits.HasPersona,
	})
}

func (c *Controller) UpdatePersona(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	limits := limitsForPlan(user.PlanType)
	if !limits.HasPersona {
		utils.JSONErr(w, http.StatusPaymentRequired, "persona builder requires PRO plan or above")
		return
	}

	var body PersonaSettings
	if err := utils.DecodeJSON(r, &body); err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid payload")
		return
	}
	body.Role = strings.TrimSpace(body.Role)
	body.BotName = strings.TrimSpace(body.BotName)
	body.Tone = strings.TrimSpace(body.Tone)
	body.Instructions = strings.TrimSpace(body.Instructions)

	// Validate role is allowed for this plan
	if body.Role != "" && body.Role != "default" {
		roleAllowed := false
		for _, r := range limits.Roles {
			if r == body.Role {
				roleAllowed = true
				break
			}
		}
		if !roleAllowed {
			utils.JSONErr(w, http.StatusForbidden, "role not available on your plan")
			return
		}
	}

	patch := map[string]interface{}{"persona": body}
	patchJSON, _ := json.Marshal(patch)

	var widgetKey string
	err := c.db.QueryRow(`UPDATE widget_keys SET widget_config = widget_config || $2::jsonb, updated_at=CURRENT_TIMESTAMP WHERE user_id=$1 RETURNING widget_key`,
		claims.UserID, string(patchJSON)).Scan(&widgetKey)
	if err != nil {
		c.logRequestError(r, "update persona failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	_ = c.redis.Del(ctx, "widget:"+widgetKey).Err()

	utils.JSONOK(w, map[string]interface{}{"success": true, "persona": body})
}
