package controller

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"konvoq-backend/utils"
)

func (c *Controller) ListFlows(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	limits := limitsForPlan(user.PlanType)
	if !limits.HasFlows {
		utils.JSONOK(w, map[string]interface{}{"success": true, "hasFlows": false, "flows": []interface{}{}})
		return
	}

	rows, err := c.db.Query(`SELECT id, name, is_active, created_at, updated_at FROM conversation_flows WHERE user_id=$1 ORDER BY updated_at DESC`, claims.UserID)
	if err != nil {
		c.logRequestError(r, "list flows query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	items := []map[string]interface{}{}
	for rows.Next() {
		var id, name string
		var isActive bool
		var created, updated time.Time
		if err := rows.Scan(&id, &name, &isActive, &created, &updated); err != nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"id": id, "name": name, "isActive": isActive,
			"createdAt": created, "updatedAt": updated,
		})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "hasFlows": true, "flows": items})
}

func (c *Controller) CreateFlow(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	limits := limitsForPlan(user.PlanType)
	if !limits.HasFlows {
		utils.JSONErr(w, http.StatusPaymentRequired, "conversation flows require PRO plan or above")
		return
	}

	var body struct {
		Name     string                 `json:"name"`
		FlowData map[string]interface{} `json:"flowData"`
		IsActive bool                   `json:"isActive"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid payload")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "New Flow"
	}
	if body.FlowData == nil {
		body.FlowData = map[string]interface{}{"nodes": []interface{}{}, "edges": []interface{}{}}
	}
	flowJSON, _ := json.Marshal(body.FlowData)

	// If setting as active, deactivate others first
	if body.IsActive {
		_, _ = c.db.Exec(`UPDATE conversation_flows SET is_active=FALSE WHERE user_id=$1`, claims.UserID)
	}

	var id string
	err := c.db.QueryRow(`
		INSERT INTO conversation_flows (user_id, name, flow_data, is_active)
		VALUES ($1, $2, $3::jsonb, $4) RETURNING id`,
		claims.UserID, name, string(flowJSON), body.IsActive).Scan(&id)
	if err != nil {
		c.logRequestError(r, "create flow failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "flow": map[string]interface{}{
		"id": id, "name": name, "flowData": body.FlowData, "isActive": body.IsActive,
	}})
}

func (c *Controller) GetFlow(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	id := chi.URLParam(r, "id")
	var name string
	var isActive bool
	var flowRaw []byte
	var created, updated time.Time
	err := c.db.QueryRow(`SELECT name, flow_data, is_active, created_at, updated_at FROM conversation_flows WHERE id=$1 AND user_id=$2`,
		id, claims.UserID).Scan(&name, &flowRaw, &isActive, &created, &updated)
	if err != nil {
		if err == sql.ErrNoRows {
			utils.JSONErr(w, http.StatusNotFound, "flow not found")
			return
		}
		c.logRequestError(r, "get flow query failed", err, "user_id", claims.UserID, "flow_id", id)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	flowData := map[string]interface{}{}
	_ = json.Unmarshal(flowRaw, &flowData)
	utils.JSONOK(w, map[string]interface{}{"success": true, "flow": map[string]interface{}{
		"id": id, "name": name, "flowData": flowData, "isActive": isActive,
		"createdAt": created, "updatedAt": updated,
	}})
}

func (c *Controller) UpdateFlow(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Name     string                 `json:"name"`
		FlowData map[string]interface{} `json:"flowData"`
		IsActive bool                   `json:"isActive"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid payload")
		return
	}

	flowJSON, _ := json.Marshal(body.FlowData)
	if body.IsActive {
		_, _ = c.db.Exec(`UPDATE conversation_flows SET is_active=FALSE WHERE user_id=$1 AND id!=$2`, claims.UserID, id)
	}

	_, err := c.db.Exec(`
		UPDATE conversation_flows SET name=$3, flow_data=$4::jsonb, is_active=$5, updated_at=CURRENT_TIMESTAMP
		WHERE id=$1 AND user_id=$2`,
		id, claims.UserID, strings.TrimSpace(body.Name), string(flowJSON), body.IsActive)
	if err != nil {
		c.logRequestError(r, "update flow failed", err, "user_id", claims.UserID, "flow_id", id)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.GetFlow(w, r, claims, UserRecord{})
}

func (c *Controller) DeleteFlow(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := c.db.Exec(`DELETE FROM conversation_flows WHERE id=$1 AND user_id=$2`, id, claims.UserID); err != nil {
		c.logRequestError(r, "delete flow failed", err, "user_id", claims.UserID, "flow_id", id)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}
