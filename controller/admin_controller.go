package controller

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"konvoq-backend/utils"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
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
	inputEmail := strings.ToLower(strings.TrimSpace(body.Email))
	inputPassword := strings.TrimSpace(body.Password)
	if inputEmail == "" || inputPassword == "" {
		utils.JSONErr(w, http.StatusBadRequest, "email and password are required")
		return
	}

	if err := c.ensureBootstrapAdmin(r.Context()); err != nil {
		c.logRequestError(r, "admin bootstrap setup failed", err)
		utils.JSONErr(w, http.StatusServiceUnavailable, "admin authentication is misconfigured")
		return
	}

	var adminID, adminEmail, adminPasswordHash, adminRole string
	var isActive bool
	err := c.db.QueryRowContext(r.Context(), `SELECT id, email, password_hash, role, is_active FROM admin_users WHERE email=$1`, inputEmail).
		Scan(&adminID, &adminEmail, &adminPasswordHash, &adminRole, &isActive)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			c.logRequestError(r, "admin login lookup failed", err, "email", inputEmail)
		}
		utils.JSONErr(w, http.StatusUnauthorized, "invalid admin credentials")
		return
	}

	if !isActive {
		utils.JSONErr(w, http.StatusForbidden, "admin account is inactive")
		return
	}
	if !adminPasswordIsBcrypt(adminPasswordHash) {
		c.logRequestError(r, "admin login hash format invalid", errors.New("password hash must use bcrypt"), "admin_id", adminID)
		utils.JSONErr(w, http.StatusServiceUnavailable, "admin authentication is misconfigured")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(adminPasswordHash), []byte(inputPassword)) != nil {
		utils.JSONErr(w, http.StatusUnauthorized, "invalid admin credentials")
		return
	}

	expiryHours := c.cfg.AdminTokenExpiryHours
	if expiryHours <= 0 {
		expiryHours = 24
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, TokenClaims{
		Type:      "admin",
		AdminID:   adminID,
		AdminRole: strings.ToLower(strings.TrimSpace(adminRole)),
		Email:     adminEmail,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	signedToken, err := token.SignedString([]byte(c.cfg.AdminJWTSecret))
	if err != nil {
		c.logRequestError(r, "admin login token signing failed", err, "admin_id", adminID)
		utils.JSONErr(w, http.StatusInternalServerError, "admin authentication failed")
		return
	}

	if _, err := c.db.ExecContext(r.Context(), `UPDATE admin_users SET last_login=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=$1`, adminID); err != nil {
		c.logRequestWarn(r, "admin login last_login update failed", err, "admin_id", adminID)
	}

	utils.JSONOK(w, map[string]interface{}{
		"success": true,
		"token":   signedToken,
		"admin": map[string]interface{}{
			"id":    adminID,
			"email": adminEmail,
			"role":  strings.ToLower(strings.TrimSpace(adminRole)),
		},
	})
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

func adminPasswordIsBcrypt(stored string) bool {
	v := strings.TrimSpace(stored)
	return strings.HasPrefix(v, "$2a$") || strings.HasPrefix(v, "$2b$") || strings.HasPrefix(v, "$2y$")
}

func normalizeAdminRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "super_admin", "admin", "support", "readonly":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return "super_admin"
	}
}

func (c *Controller) ensureBootstrapAdmin(ctx context.Context) error {
	email := strings.ToLower(strings.TrimSpace(c.cfg.AdminBootstrapEmail))
	password := strings.TrimSpace(c.cfg.AdminBootstrapPassword)
	if email == "" || password == "" {
		return nil
	}

	hash := password
	if !adminPasswordIsBcrypt(hash) {
		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		hash = string(hashed)
	}

	_, err := c.db.ExecContext(ctx, `
		WITH existing_admins AS (
			SELECT COUNT(*) AS total FROM admin_users
		)
		INSERT INTO admin_users (email, password_hash, role, is_active)
		SELECT $1, $2, $3, TRUE
		FROM existing_admins
		WHERE total = 0
		ON CONFLICT (email) DO NOTHING
	`, email, hash, normalizeAdminRole(c.cfg.AdminBootstrapRole))
	return err
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
