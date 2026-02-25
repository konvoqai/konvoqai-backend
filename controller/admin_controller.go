package controller

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"konvoq-backend/utils"

	"github.com/golang-jwt/jwt/v5"
)

func (c *Controller) AdminLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if body.Email != c.cfg.AdminEmail || body.Password != c.cfg.AdminPassword {
		utils.JSONErr(w, http.StatusUnauthorized, "invalid admin credentials")
		return
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, TokenClaims{
		Type: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	s, _ := token.SignedString([]byte(c.cfg.AdminJWTSecret))
	utils.JSONOK(w, map[string]interface{}{"success": true, "token": s})
}

func (c *Controller) AdminDashboard(w http.ResponseWriter, r *http.Request) {
	var users, sessions, leads int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		c.logRequestWarn(r, "admin dashboard users count failed", err)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE is_revoked=FALSE`).Scan(&sessions); err != nil {
		c.logRequestWarn(r, "admin dashboard sessions count failed", err)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM leads`).Scan(&leads); err != nil {
		c.logRequestWarn(r, "admin dashboard leads count failed", err)
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "dashboard": map[string]int{"users": users, "sessions": sessions, "leads": leads}})
}

func (c *Controller) AdminInsights(w http.ResponseWriter, r *http.Request) {
	rows, err := c.db.Query(`SELECT plan_type,COUNT(*) FROM users GROUP BY plan_type`)
	if err != nil {
		c.logRequestError(r, "admin insights query failed", err)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	plans := map[string]int{}
	for rows.Next() {
		var p string
		var cnt int
		if err := rows.Scan(&p, &cnt); err != nil {
			c.logRequestWarn(r, "admin insights row scan failed", err)
			continue
		}
		plans[p] = cnt
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "insights": map[string]interface{}{"plans": plans}})
}

func (c *Controller) AdminUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := c.db.Query(`SELECT id,email,is_verified,plan_type,login_count,profile_completed,created_at,last_login FROM users ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		c.logRequestError(r, "admin users query failed", err)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id, email, plan string
		var verified, profile bool
		var login int
		var created time.Time
		var lastLogin sql.NullTime
		if err := rows.Scan(&id, &email, &verified, &plan, &login, &profile, &created, &lastLogin); err != nil {
			c.logRequestWarn(r, "admin users row scan failed", err)
			continue
		}
		items = append(items, map[string]interface{}{
			"id": id, "email": email, "isVerified": verified, "plan_type": plan,
			"loginCount": login, "profileCompleted": profile, "createdAt": created,
			"lastLogin": utils.NullTime(lastLogin),
		})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "users": items})
}

func (c *Controller) AdminResetUsage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"userId"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.UserID) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "userId is required")
		return
	}
	if _, err := c.db.Exec(`UPDATE users SET conversations_used=0,plan_reset_date=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, body.UserID); err != nil {
		c.logRequestError(r, "admin reset usage update failed", err, "target_user_id", body.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) AdminForceLogout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"userId"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.UserID) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "userId is required")
		return
	}
	if _, err := c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE user_id=$1`, body.UserID); err != nil {
		c.logRequestError(r, "admin force logout update failed", err, "target_user_id", body.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) AdminSetPlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"userId"`
		Plan   string `json:"plan"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || body.UserID == "" || body.Plan == "" {
		utils.JSONErr(w, http.StatusBadRequest, "userId and plan are required")
		return
	}
	limit := interface{}(int64(100))
	if body.Plan == "basic" {
		limit = int64(1000)
	} else if body.Plan == "enterprise" {
		limit = nil
	}
	if _, err := c.db.Exec(`UPDATE users SET plan_type=$2,conversations_limit=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, body.UserID, body.Plan, limit); err != nil {
		c.logRequestError(r, "admin set plan update failed", err, "target_user_id", body.UserID, "plan", body.Plan)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}
