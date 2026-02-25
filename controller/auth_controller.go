package controller

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	"konvoq-backend/utils"
)

func (c *Controller) AuthenticateUser(r *http.Request) (TokenClaims, UserRecord, error) {
	var emptyClaims TokenClaims
	var emptyUser UserRecord
	raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if raw == "" {
		if ck, err := r.Cookie("witzo_access_token"); err == nil {
			raw = ck.Value
		}
	}
	if raw == "" {
		return emptyClaims, emptyUser, errors.New("authentication required")
	}
	claims := &TokenClaims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(c.cfg.JWTSecret), nil
	})
	if err != nil || !tok.Valid || claims.Type != "access" {
		return emptyClaims, emptyUser, errors.New("invalid access token")
	}
	h := utils.HashToken(raw)
	row := c.db.QueryRow(`SELECT u.id,u.email,u.is_verified,u.plan_type,u.conversations_used,u.conversations_limit,u.plan_reset_date,
		u.login_count,u.full_name,u.company_name,u.phone_number,u.country,u.job_title,u.industry,u.company_website,
		u.profile_completed,u.profile_prompt_required_at,u.profile_completed_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id=$1 AND s.user_id=$2 AND s.access_token=$3 AND s.is_revoked=FALSE`, claims.SessionID, claims.UserID, h)
	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return emptyClaims, emptyUser, errors.New("session not found or revoked")
		}
		return emptyClaims, emptyUser, err
	}
	return *claims, user, nil
}

func (c *Controller) ValidateAdminRequest(r *http.Request) error {
	raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if raw == "" {
		return errors.New("admin authentication required")
	}
	claims := &TokenClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(c.cfg.AdminJWTSecret), nil
	})
	if err != nil || !token.Valid || claims.Type != "admin" {
		return errors.New("invalid admin token")
	}
	return nil
}

func (c *Controller) RequireCSRF(r *http.Request) error {
	token := r.Header.Get("X-CSRF-Token")
	if token == "" {
		return errors.New("missing csrf token")
	}
	cookie, err := r.Cookie("csrf_token")
	if err != nil || cookie.Value == "" || cookie.Value != token {
		return errors.New("invalid csrf token")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	exists, err := c.redis.Exists(ctx, "csrf:"+token).Result()
	if err != nil || exists == 0 {
		return errors.New("invalid csrf token")
	}
	return nil
}

func (c *Controller) createToken(userID, email string, sessionID int64, typ string, secret string, ttl time.Duration) (string, time.Time, error) {
	exp := time.Now().Add(ttl)
	claims := TokenClaims{
		UserID: userID, Email: email, SessionID: sessionID, Type: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return s, exp, nil
}

func (c *Controller) GetCSRFToken(w http.ResponseWriter, r *http.Request) {
	token := utils.RandomID("csrf")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	_ = c.redis.Set(ctx, "csrf:"+token, "1", 24*time.Hour).Err()
	http.SetCookie(w, &http.Cookie{Name: "csrf_token", Value: token, Path: "/", HttpOnly: false, Expires: time.Now().Add(24 * time.Hour)})
	utils.JSONOK(w, map[string]interface{}{"success": true, "csrfToken": token})
}

func (c *Controller) RequestCode(w http.ResponseWriter, r *http.Request) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Email) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "email is required")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	tx, err := c.db.BeginTx(r.Context(), nil)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	var userID string
	err = tx.QueryRow(`SELECT id FROM users WHERE email=$1`, email).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRow(`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&userID)
	}
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	var reqCount int
	_ = tx.QueryRow(`SELECT COUNT(*) FROM verification_codes WHERE user_id=$1 AND created_at > NOW() - INTERVAL '1 hour' AND is_used=FALSE`, userID).Scan(&reqCount)
	if reqCount >= 3 {
		utils.JSONErr(w, http.StatusTooManyRequests, "Too many verification code requests. Please try again later.")
		return
	}

	_, _ = tx.Exec(`UPDATE verification_codes SET is_used=TRUE WHERE user_id=$1 AND is_used=FALSE`, userID)
	code := utils.RandomCode()
	expires := time.Now().Add(time.Duration(c.cfg.VerifyCodeMinutes) * time.Minute)
	_, err = tx.Exec(`INSERT INTO verification_codes (user_id,code,expires_at) VALUES ($1,$2,$3)`, userID, code, expires)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "failed to create code")
		return
	}
	if err := tx.Commit(); err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.sendVerificationEmail(email, code)
	utils.JSONOK(w, map[string]interface{}{"success": true, "message": "Verification code sent", "devCode": code, "expiresIn": c.cfg.VerifyCodeMinutes * 60})
}

func (c *Controller) VerifyCode(w http.ResponseWriter, r *http.Request) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || body.Email == "" || body.Code == "" {
		utils.JSONErr(w, http.StatusBadRequest, "email and code are required")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	firstSignup := false
	tx, err := c.db.BeginTx(r.Context(), nil)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	var user UserRecord
	row := tx.QueryRow(`SELECT id,email,is_verified,plan_type,conversations_used,conversations_limit,plan_reset_date,
		login_count,full_name,company_name,phone_number,country,job_title,industry,company_website,
		profile_completed,profile_prompt_required_at,profile_completed_at
		FROM users WHERE email=$1`, email)
	user, err = scanUser(row)
	if err != nil {
		utils.JSONErr(w, http.StatusUnauthorized, "invalid email or code")
		return
	}
	firstSignup = !user.IsVerified

	var vcID int64
	var dbCode string
	var attempts int
	var expiresAt time.Time
	err = tx.QueryRow(`SELECT id,code,attempts,expires_at FROM verification_codes WHERE user_id=$1 AND is_used=FALSE ORDER BY created_at DESC LIMIT 1`, user.ID).Scan(&vcID, &dbCode, &attempts, &expiresAt)
	if err != nil {
		utils.JSONErr(w, http.StatusUnauthorized, "No valid verification code found")
		return
	}
	if time.Now().After(expiresAt) {
		_, _ = tx.Exec(`UPDATE verification_codes SET is_used=TRUE WHERE id=$1`, vcID)
		_ = tx.Commit()
		utils.JSONErr(w, http.StatusUnauthorized, "Verification code expired")
		return
	}
	if attempts >= c.cfg.MaxVerifyAttempts {
		_, _ = tx.Exec(`UPDATE verification_codes SET is_used=TRUE WHERE id=$1`, vcID)
		_ = tx.Commit()
		utils.JSONErr(w, http.StatusUnauthorized, "Maximum verification attempts exceeded")
		return
	}
	if dbCode != body.Code {
		_, _ = tx.Exec(`UPDATE verification_codes SET attempts=attempts+1 WHERE id=$1`, vcID)
		_ = tx.Commit()
		utils.JSONErr(w, http.StatusUnauthorized, "Invalid verification code")
		return
	}
	_, _ = tx.Exec(`UPDATE verification_codes SET is_used=TRUE WHERE id=$1`, vcID)
	_, err = tx.Exec(`UPDATE users SET is_verified=TRUE,last_login=CURRENT_TIMESTAMP,login_count=login_count+1,
		profile_prompt_required_at = CASE WHEN (login_count + 1) > 3 AND profile_completed=FALSE THEN COALESCE(profile_prompt_required_at, CURRENT_TIMESTAMP) ELSE profile_prompt_required_at END
		WHERE id=$1`, user.ID)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	var sessionID int64
	err = tx.QueryRow(`INSERT INTO sessions (user_id,access_token,refresh_token,access_token_expires_at,refresh_token_expires_at,ip_address,user_agent)
		VALUES ($1,'pending','pending',NOW()+INTERVAL '15 minutes',NOW()+INTERVAL '7 days',$2,$3) RETURNING id`, user.ID, r.RemoteAddr, r.UserAgent()).Scan(&sessionID)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "failed creating session")
		return
	}

	accessToken, accessExp, err := c.createToken(user.ID, user.Email, sessionID, "access", c.cfg.JWTSecret, time.Duration(c.cfg.AccessTokenMinutes)*time.Minute)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "token error")
		return
	}
	refreshToken, refreshExp, err := c.createToken(user.ID, user.Email, sessionID, "refresh", c.cfg.JWTRefreshSecret, time.Duration(c.cfg.RefreshTokenDays)*24*time.Hour)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "token error")
		return
	}
	_, err = tx.Exec(`UPDATE sessions SET access_token=$1,refresh_token=$2,access_token_expires_at=$3,refresh_token_expires_at=$4 WHERE id=$5`,
		utils.HashToken(accessToken), utils.HashToken(refreshToken), accessExp, refreshExp, sessionID)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "session update failed")
		return
	}
	row = tx.QueryRow(`SELECT id,email,is_verified,plan_type,conversations_used,conversations_limit,plan_reset_date,
		login_count,full_name,company_name,phone_number,country,job_title,industry,company_website,
		profile_completed,profile_prompt_required_at,profile_completed_at
		FROM users WHERE id=$1`, user.ID)
	updatedUser, err := scanUser(row)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := tx.Commit(); err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	http.SetCookie(w, &http.Cookie{Name: "witzo_access_token", Value: accessToken, HttpOnly: true, Path: "/", Expires: accessExp})
	http.SetCookie(w, &http.Cookie{Name: "witzo_refresh_token", Value: refreshToken, HttpOnly: true, Path: "/", Expires: refreshExp})
	if firstSignup {
		c.sendWelcomeEmail(email)
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "message": "Login successful", "accessToken": accessToken, "refreshToken": refreshToken, "user": userResponse(updatedUser, sessionID)})
}

func (c *Controller) RefreshToken(w http.ResponseWriter, r *http.Request) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	refresh := ""
	if ck, err := r.Cookie("witzo_refresh_token"); err == nil {
		refresh = ck.Value
	}
	if refresh == "" {
		var body struct {
			RefreshToken string `json:"refreshToken"`
		}
		_ = utils.DecodeJSON(r, &body)
		refresh = body.RefreshToken
	}
	if refresh == "" {
		utils.JSONErr(w, http.StatusUnauthorized, "refresh token required")
		return
	}
	claims := &TokenClaims{}
	tok, err := jwt.ParseWithClaims(refresh, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(c.cfg.JWTRefreshSecret), nil
	})
	if err != nil || !tok.Valid || claims.Type != "refresh" {
		utils.JSONErr(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	var userEmail string
	err = c.db.QueryRow(`SELECT u.email FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.id=$1 AND s.user_id=$2 AND s.refresh_token=$3 AND s.is_revoked=FALSE AND s.refresh_token_expires_at > CURRENT_TIMESTAMP`,
		claims.SessionID, claims.UserID, utils.HashToken(refresh)).Scan(&userEmail)
	if err != nil {
		utils.JSONErr(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}
	newAccess, accessExp, err := c.createToken(claims.UserID, userEmail, claims.SessionID, "access", c.cfg.JWTSecret, time.Duration(c.cfg.AccessTokenMinutes)*time.Minute)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "token error")
		return
	}
	newRefresh, refreshExp, err := c.createToken(claims.UserID, userEmail, claims.SessionID, "refresh", c.cfg.JWTRefreshSecret, time.Duration(c.cfg.RefreshTokenDays)*24*time.Hour)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "token error")
		return
	}
	_, err = c.db.Exec(`UPDATE sessions SET access_token=$1,refresh_token=$2,access_token_expires_at=$3,refresh_token_expires_at=$4,updated_at=CURRENT_TIMESTAMP WHERE id=$5`,
		utils.HashToken(newAccess), utils.HashToken(newRefresh), accessExp, refreshExp, claims.SessionID)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "witzo_access_token", Value: newAccess, HttpOnly: true, Path: "/", Expires: accessExp})
	http.SetCookie(w, &http.Cookie{Name: "witzo_refresh_token", Value: newRefresh, HttpOnly: true, Path: "/", Expires: refreshExp})
	utils.JSONOK(w, map[string]interface{}{"success": true, "accessToken": newAccess, "newRefreshToken": newRefresh})
}

func (c *Controller) Logout(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	_, _ = c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE id=$1`, claims.SessionID)
	http.SetCookie(w, &http.Cookie{Name: "witzo_access_token", Value: "", HttpOnly: true, Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "witzo_refresh_token", Value: "", HttpOnly: true, Path: "/", MaxAge: -1})
	utils.JSONOK(w, map[string]interface{}{"success": true, "message": "Logged out"})
}

func (c *Controller) Me(w http.ResponseWriter, _ *http.Request, claims TokenClaims, user UserRecord) {
	utils.JSONOK(w, map[string]interface{}{"success": true, "user": userResponse(user, claims.SessionID)})
}

func (c *Controller) ValidateSession(w http.ResponseWriter, _ *http.Request, claims TokenClaims, user UserRecord) {
	utils.JSONOK(w, map[string]interface{}{"success": true, "valid": true, "user": userResponse(user, claims.SessionID)})
}

func (c *Controller) ProfileStatus(w http.ResponseWriter, _ *http.Request, claims TokenClaims, user UserRecord) {
	utils.JSONOK(w, map[string]interface{}{"success": true, "profileCompleted": user.ProfileCompleted, "plan": user.PlanType, "user": userResponse(user, claims.SessionID)})
}

func (c *Controller) UpdateProfile(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body struct {
		FullName       string `json:"full_name"`
		CompanyName    string `json:"company_name"`
		PhoneNumber    string `json:"phone_number"`
		Country        string `json:"country"`
		JobTitle       string `json:"job_title"`
		Industry       string `json:"industry"`
		CompanyWebsite string `json:"company_website"`
		Name           string `json:"name"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if body.Name != "" && body.FullName == "" {
		body.FullName = body.Name
	}
	if strings.TrimSpace(body.CompanyWebsite) != "" && !strings.HasPrefix(strings.ToLower(body.CompanyWebsite), "http") {
		body.CompanyWebsite = "https://" + strings.TrimSpace(body.CompanyWebsite)
	}
	_, err := c.db.Exec(`UPDATE users SET full_name=$2,company_name=$3,phone_number=$4,country=$5,job_title=$6,industry=$7,company_website=$8,
		profile_completed = ($2 IS NOT NULL AND $2 <> '' AND $3 IS NOT NULL AND $3 <> '' AND $4 IS NOT NULL AND $4 <> '' AND $5 IS NOT NULL AND $5 <> '' AND $6 IS NOT NULL AND $6 <> '' AND $7 IS NOT NULL AND $7 <> '' AND $8 IS NOT NULL AND $8 <> ''),
		profile_completed_at = CASE WHEN ($2 IS NOT NULL AND $2 <> '' AND $3 IS NOT NULL AND $3 <> '' AND $4 IS NOT NULL AND $4 <> '' AND $5 IS NOT NULL AND $5 <> '' AND $6 IS NOT NULL AND $6 <> '' AND $7 IS NOT NULL AND $7 <> '' AND $8 IS NOT NULL AND $8 <> '') THEN COALESCE(profile_completed_at, CURRENT_TIMESTAMP) ELSE profile_completed_at END,
		updated_at=CURRENT_TIMESTAMP WHERE id=$1`,
		claims.UserID, utils.Nullable(body.FullName), utils.Nullable(body.CompanyName), utils.Nullable(body.PhoneNumber),
		utils.Nullable(body.Country), utils.Nullable(body.JobTitle), utils.Nullable(body.Industry), utils.Nullable(body.CompanyWebsite))
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	row := c.db.QueryRow(`SELECT id,email,is_verified,plan_type,conversations_used,conversations_limit,plan_reset_date,
		login_count,full_name,company_name,phone_number,country,job_title,industry,company_website,
		profile_completed,profile_prompt_required_at,profile_completed_at FROM users WHERE id=$1`, claims.UserID)
	u, err := scanUser(row)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "user": userResponse(u, claims.SessionID)})
}

func (c *Controller) GetSessions(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,user_id,created_at,access_token_expires_at,refresh_token_expires_at,ip_address,user_agent,is_revoked FROM sessions WHERE user_id=$1 ORDER BY created_at DESC`, claims.UserID)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int64
		var uid string
		var created, accessExp, refreshExp time.Time
		var ip, ua sql.NullString
		var revoked bool
		_ = rows.Scan(&id, &uid, &created, &accessExp, &refreshExp, &ip, &ua, &revoked)
		items = append(items, map[string]interface{}{
			"id": id, "userId": uid, "createdAt": created,
			"accessTokenExpiresAt": accessExp, "refreshTokenExpiresAt": refreshExp,
			"ipAddress": utils.NullString(ip), "userAgent": utils.NullString(ua), "isRevoked": revoked,
		})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "sessions": items})
}

func (c *Controller) RevokeSession(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	sid := chi.URLParam(r, "id")
	_, err := c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE id=$1 AND user_id=$2`, sid, claims.UserID)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) RevokeAllOtherSessions(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	_, err := c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE user_id=$1 AND id <> $2`, claims.UserID, claims.SessionID)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) LogoutAll(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	_, err := c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE user_id=$1`, claims.UserID)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "witzo_access_token", Value: "", HttpOnly: true, Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "witzo_refresh_token", Value: "", HttpOnly: true, Path: "/", MaxAge: -1})
	utils.JSONOK(w, map[string]interface{}{"success": true})
}
