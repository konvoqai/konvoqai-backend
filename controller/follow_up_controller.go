package controller

import (
	"net/http"
	"strings"
	"time"

	"konvoq-backend/utils"
)

type FollowUpConfig struct {
	IsActive        bool   `json:"isActive"`
	DelayHours      int    `json:"delayHours"`
	TriggerEvent    string `json:"triggerEvent"`
	TemplateSubject string `json:"templateSubject"`
	TemplateBody    string `json:"templateBody"`
}

func (c *Controller) GetFollowUpConfig(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	limits := limitsForPlan(user.PlanType)
	if !limits.HasFollowUp {
		utils.JSONOK(w, map[string]interface{}{
			"success":      true,
			"hasFollowUp":  false,
			"config":       nil,
		})
		return
	}

	var cfg FollowUpConfig
	err := c.db.QueryRow(`
		SELECT is_active, delay_hours, trigger_event, template_subject, template_body
		FROM follow_up_configs WHERE user_id=$1`, claims.UserID).
		Scan(&cfg.IsActive, &cfg.DelayHours, &cfg.TriggerEvent, &cfg.TemplateSubject, &cfg.TemplateBody)
	if err != nil {
		// Not configured yet — return defaults
		cfg = FollowUpConfig{
			IsActive:        false,
			DelayHours:      24,
			TriggerEvent:    "lead_created",
			TemplateSubject: "Following up on your inquiry",
			TemplateBody:    "Hi {{name}},\n\nThank you for reaching out. I wanted to follow up on your recent inquiry.\n\nBest regards",
		}
	}

	utils.JSONOK(w, map[string]interface{}{
		"success":     true,
		"hasFollowUp": true,
		"config":      cfg,
	})
}

func (c *Controller) UpdateFollowUpConfig(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	limits := limitsForPlan(user.PlanType)
	if !limits.HasFollowUp {
		utils.JSONErr(w, http.StatusPaymentRequired, "auto follow-up requires PRO plan or above")
		return
	}

	var body FollowUpConfig
	if err := utils.DecodeJSON(r, &body); err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid payload")
		return
	}
	body.TemplateSubject = strings.TrimSpace(body.TemplateSubject)
	body.TemplateBody = strings.TrimSpace(body.TemplateBody)
	if body.DelayHours <= 0 {
		body.DelayHours = 24
	}
	if strings.TrimSpace(body.TriggerEvent) == "" {
		body.TriggerEvent = "lead_created"
	}

	_, err := c.db.Exec(`
		INSERT INTO follow_up_configs (user_id, is_active, delay_hours, trigger_event, template_subject, template_body, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id) DO UPDATE SET
			is_active=$2, delay_hours=$3, trigger_event=$4,
			template_subject=$5, template_body=$6, updated_at=$7`,
		claims.UserID, body.IsActive, body.DelayHours, body.TriggerEvent,
		body.TemplateSubject, body.TemplateBody, time.Now())
	if err != nil {
		c.logRequestError(r, "update follow-up config failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	utils.JSONOK(w, map[string]interface{}{"success": true, "config": body})
}
