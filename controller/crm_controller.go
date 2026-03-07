package controller

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"konvoq-backend/utils"
)

func (c *Controller) CRMPipeline(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	limits := limitsForPlan(user.PlanType)
	// BASIC gets read-only pipeline view; PRO+ gets full access
	stages := []string{"new", "contacted", "qualified", "proposal", "won", "lost"}
	pipeline := map[string]int{}
	for _, s := range stages {
		pipeline[s] = 0
	}

	rows, err := c.db.Query(`
		SELECT pipeline_stage, COUNT(*) FROM leads
		WHERE user_id=$1
		GROUP BY pipeline_stage`, claims.UserID)
	if err != nil {
		c.logRequestError(r, "crm pipeline query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var stage string
		var count int
		if err := rows.Scan(&stage, &count); err == nil {
			pipeline[stage] = count
		}
	}

	// Get leads per stage with contact info for kanban
	leadsPerStage := map[string][]map[string]interface{}{}
	for _, s := range stages {
		leadsPerStage[s] = []map[string]interface{}{}
	}

	leadRows, err := c.db.Query(`
		SELECT id, name, email, phone, pipeline_stage, crm_notes, next_follow_up_at, tags, created_at, updated_at
		FROM leads WHERE user_id=$1 ORDER BY created_at DESC`, claims.UserID)
	if err == nil {
		defer leadRows.Close()
		for leadRows.Next() {
			var id, stage string
			var name, email, phone, notes sql.NullString
			var nextFollowUp sql.NullTime
			var tagsRaw []byte
			var created, updated time.Time
			if err := leadRows.Scan(&id, &name, &email, &phone, &stage, &notes, &nextFollowUp, &tagsRaw, &created, &updated); err != nil {
				continue
			}
			tags := []string{}
			if len(tagsRaw) > 0 {
				// tags stored as postgres text array, parse basic
				_ = tagsRaw
			}
			lead := map[string]interface{}{
				"id": id, "name": utils.NullString(name), "email": utils.NullString(email),
				"phone": utils.NullString(phone), "pipelineStage": stage,
				"crmNotes": utils.NullString(notes), "tags": tags,
				"createdAt": created, "updatedAt": updated,
			}
			if nextFollowUp.Valid {
				lead["nextFollowUpAt"] = nextFollowUp.Time
			}
			if _, ok := leadsPerStage[stage]; ok {
				leadsPerStage[stage] = append(leadsPerStage[stage], lead)
			}
		}
	}

	utils.JSONOK(w, map[string]interface{}{
		"success":       true,
		"pipeline":      pipeline,
		"leads":         leadsPerStage,
		"hasCRM":        limits.HasCRM,
		"readOnly":      !limits.HasCRM, // BASIC gets read-only
	})
}

func (c *Controller) UpdateLeadPipeline(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	limits := limitsForPlan(user.PlanType)
	// At minimum BASIC can view, but only PRO+ can edit
	if !limits.HasCRM {
		utils.JSONErr(w, http.StatusPaymentRequired, "CRM editing requires PRO plan or above")
		return
	}

	id := chi.URLParam(r, "id")
	var body struct {
		PipelineStage  string   `json:"pipelineStage"`
		CRMNotes       string   `json:"crmNotes"`
		NextFollowUpAt string   `json:"nextFollowUpAt"`
		Tags           []string `json:"tags"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid payload")
		return
	}

	validStages := map[string]bool{"new": true, "contacted": true, "qualified": true, "proposal": true, "won": true, "lost": true}
	if body.PipelineStage != "" && !validStages[body.PipelineStage] {
		utils.JSONErr(w, http.StatusBadRequest, "invalid pipeline stage")
		return
	}

	nextFollowUp := sql.NullTime{}
	if strings.TrimSpace(body.NextFollowUpAt) != "" {
		if t, err := time.Parse(time.RFC3339, body.NextFollowUpAt); err == nil {
			nextFollowUp = sql.NullTime{Time: t, Valid: true}
		}
	}

	_, err := c.db.Exec(`
		UPDATE leads SET
			pipeline_stage = CASE WHEN $3 != '' THEN $3 ELSE pipeline_stage END,
			crm_notes = COALESCE($4, crm_notes),
			next_follow_up_at = $5,
			updated_at = CURRENT_TIMESTAMP
		WHERE id=$1 AND user_id=$2`,
		id, claims.UserID,
		strings.TrimSpace(body.PipelineStage),
		utils.Nullable(strings.TrimSpace(body.CRMNotes)),
		nextFollowUp)
	if err != nil {
		c.logRequestError(r, "update lead pipeline failed", err, "user_id", claims.UserID, "lead_id", id)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	utils.JSONOK(w, map[string]interface{}{"success": true})
}
