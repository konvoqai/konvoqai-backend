package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func (c *Handler) AuthenticateUser(r *http.Request) (TokenClaims, UserRecord, error) {
	var emptyClaims TokenClaims
	var emptyUser UserRecord
	raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if raw == "" {
		if c, err := r.Cookie("witzo_access_token"); err == nil {
			raw = c.Value
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
	h := hashToken(raw)
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

func (c *Handler) ValidateAdminRequest(r *http.Request) error {
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

func (c *Handler) RequireCSRF(r *http.Request) error {
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

func (c *Handler) createToken(userID, email string, sessionID int64, typ string, secret string, ttl time.Duration) (string, time.Time, error) {
	exp := time.Now().Add(ttl)
	claims := TokenClaims{UserID: userID, Email: email, SessionID: sessionID, Type: typ, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(exp), IssuedAt: jwt.NewNumericDate(time.Now())}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return s, exp, nil
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func randomID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return prefix + "_" + hex.EncodeToString(b)
}

func randomCode() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "123456"
	}
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	return fmt.Sprintf("%06d", n%1000000)
}
func scanUser(row *sql.Row) (UserRecord, error) {
	var u UserRecord
	err := row.Scan(&u.ID, &u.Email, &u.IsVerified, &u.PlanType, &u.ConversationsUsed, &u.ConversationsLimit, &u.PlanResetDate,
		&u.LoginCount, &u.FullName, &u.CompanyName, &u.PhoneNumber, &u.Country, &u.JobTitle, &u.Industry, &u.CompanyWebsite,
		&u.ProfileCompleted, &u.ProfilePromptRequired, &u.ProfileCompletedAt)
	return u, err
}

func userResponse(u UserRecord, sessionID int64) map[string]interface{} {
	return map[string]interface{}{
		"id":                        u.ID,
		"email":                     u.Email,
		"isVerified":                u.IsVerified,
		"plan_type":                 u.PlanType,
		"sessionId":                 sessionID,
		"loginCount":                u.LoginCount,
		"fullName":                  nullString(u.FullName),
		"companyName":               nullString(u.CompanyName),
		"phoneNumber":               nullString(u.PhoneNumber),
		"country":                   nullString(u.Country),
		"jobTitle":                  nullString(u.JobTitle),
		"industry":                  nullString(u.Industry),
		"companyWebsite":            nullString(u.CompanyWebsite),
		"profileCompleted":          u.ProfileCompleted,
		"requiresProfileCompletion": !u.ProfileCompleted && u.LoginCount > 3,
		"profilePromptRequiredAt":   nullTime(u.ProfilePromptRequired),
		"profileCompletedAt":        nullTime(u.ProfileCompletedAt),
	}
}

func nullString(v sql.NullString) interface{} {
	if !v.Valid {
		return nil
	}
	return v.String
}

func nullTime(v sql.NullTime) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Time
}

func decodeJSON(r *http.Request, out interface{}) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

func jsonOK(w http.ResponseWriter, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

func jsonErr(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": message})
}

func readUploadedFile(r *http.Request, field string) (string, int64, string, error) {
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		return "", 0, "", errors.New("invalid multipart form")
	}
	files := r.MultipartForm.File[field]
	if len(files) == 0 {
		return "", 0, "", fmt.Errorf("missing '%s' file", field)
	}
	h := files[0]
	return h.Filename, h.Size, h.Header.Get("Content-Type"), nil
}

func docFromHeader(h *multipart.FileHeader) (string, int64, string) {
	return h.Filename, h.Size, h.Header.Get("Content-Type")
}

var startedAt = time.Now()

func (c *Handler) GetCSRFToken(w http.ResponseWriter, r *http.Request) {
	token := randomID("csrf")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	_ = c.redis.Set(ctx, "csrf:"+token, "1", 24*time.Hour).Err()
	http.SetCookie(w, &http.Cookie{Name: "csrf_token", Value: token, Path: "/", HttpOnly: false, Expires: time.Now().Add(24 * time.Hour)})
	jsonOK(w, map[string]interface{}{"success": true, "csrfToken": token})
}

func (c *Handler) RequestCode(w http.ResponseWriter, r *http.Request) {
	if err := c.RequireCSRF(r); err != nil {
		jsonErr(w, http.StatusForbidden, err.Error())
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Email) == "" {
		jsonErr(w, http.StatusBadRequest, "email is required")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	tx, err := c.db.BeginTx(r.Context(), nil)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	var userID string
	err = tx.QueryRow(`SELECT id FROM users WHERE email=$1`, email).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRow(`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&userID)
	}
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}

	var reqCount int
	_ = tx.QueryRow(`SELECT COUNT(*) FROM verification_codes WHERE user_id=$1 AND created_at > NOW() - INTERVAL '1 hour' AND is_used=FALSE`, userID).Scan(&reqCount)
	if reqCount >= 3 {
		jsonErr(w, http.StatusTooManyRequests, "Too many verification code requests. Please try again later.")
		return
	}

	_, _ = tx.Exec(`UPDATE verification_codes SET is_used=TRUE WHERE user_id=$1 AND is_used=FALSE`, userID)
	code := randomCode()
	expires := time.Now().Add(time.Duration(c.cfg.VerifyCodeMinutes) * time.Minute)
	_, err = tx.Exec(`INSERT INTO verification_codes (user_id,code,expires_at) VALUES ($1,$2,$3)`, userID, code, expires)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "failed to create code")
		return
	}
	if err := tx.Commit(); err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.sendVerificationEmail(email, code)
	jsonOK(w, map[string]interface{}{"success": true, "message": "Verification code sent", "devCode": code, "expiresIn": c.cfg.VerifyCodeMinutes * 60})
}

func (c *Handler) VerifyCode(w http.ResponseWriter, r *http.Request) {
	if err := c.RequireCSRF(r); err != nil {
		jsonErr(w, http.StatusForbidden, err.Error())
		return
	}
	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Email == "" || body.Code == "" {
		jsonErr(w, http.StatusBadRequest, "email and code are required")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	firstSignup := false
	tx, err := c.db.BeginTx(r.Context(), nil)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
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
		jsonErr(w, http.StatusUnauthorized, "invalid email or code")
		return
	}
	firstSignup = !user.IsVerified

	var vcID int64
	var dbCode string
	var attempts int
	var expiresAt time.Time
	err = tx.QueryRow(`SELECT id,code,attempts,expires_at FROM verification_codes WHERE user_id=$1 AND is_used=FALSE ORDER BY created_at DESC LIMIT 1`, user.ID).Scan(&vcID, &dbCode, &attempts, &expiresAt)
	if err != nil {
		jsonErr(w, http.StatusUnauthorized, "No valid verification code found")
		return
	}
	if time.Now().After(expiresAt) {
		_, _ = tx.Exec(`UPDATE verification_codes SET is_used=TRUE WHERE id=$1`, vcID)
		_ = tx.Commit()
		jsonErr(w, http.StatusUnauthorized, "Verification code expired")
		return
	}
	if attempts >= c.cfg.MaxVerifyAttempts {
		_, _ = tx.Exec(`UPDATE verification_codes SET is_used=TRUE WHERE id=$1`, vcID)
		_ = tx.Commit()
		jsonErr(w, http.StatusUnauthorized, "Maximum verification attempts exceeded")
		return
	}
	if dbCode != body.Code {
		_, _ = tx.Exec(`UPDATE verification_codes SET attempts=attempts+1 WHERE id=$1`, vcID)
		_ = tx.Commit()
		jsonErr(w, http.StatusUnauthorized, "Invalid verification code")
		return
	}
	_, _ = tx.Exec(`UPDATE verification_codes SET is_used=TRUE WHERE id=$1`, vcID)
	_, err = tx.Exec(`UPDATE users SET is_verified=TRUE,last_login=CURRENT_TIMESTAMP,login_count=login_count+1,
		profile_prompt_required_at = CASE WHEN (login_count + 1) > 3 AND profile_completed=FALSE THEN COALESCE(profile_prompt_required_at, CURRENT_TIMESTAMP) ELSE profile_prompt_required_at END
		WHERE id=$1`, user.ID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}

	var sessionID int64
	err = tx.QueryRow(`INSERT INTO sessions (user_id,access_token,refresh_token,access_token_expires_at,refresh_token_expires_at,ip_address,user_agent)
		VALUES ($1,'pending','pending',NOW()+INTERVAL '15 minutes',NOW()+INTERVAL '7 days',$2,$3) RETURNING id`, user.ID, r.RemoteAddr, r.UserAgent()).Scan(&sessionID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "failed creating session")
		return
	}

	accessToken, accessExp, err := c.createToken(user.ID, user.Email, sessionID, "access", c.cfg.JWTSecret, time.Duration(c.cfg.AccessTokenMinutes)*time.Minute)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "token error")
		return
	}
	refreshToken, refreshExp, err := c.createToken(user.ID, user.Email, sessionID, "refresh", c.cfg.JWTRefreshSecret, time.Duration(c.cfg.RefreshTokenDays)*24*time.Hour)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "token error")
		return
	}
	_, err = tx.Exec(`UPDATE sessions SET access_token=$1,refresh_token=$2,access_token_expires_at=$3,refresh_token_expires_at=$4 WHERE id=$5`, hashToken(accessToken), hashToken(refreshToken), accessExp, refreshExp, sessionID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "session update failed")
		return
	}
	row = tx.QueryRow(`SELECT id,email,is_verified,plan_type,conversations_used,conversations_limit,plan_reset_date,
		login_count,full_name,company_name,phone_number,country,job_title,industry,company_website,
		profile_completed,profile_prompt_required_at,profile_completed_at
		FROM users WHERE id=$1`, user.ID)
	updatedUser, err := scanUser(row)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := tx.Commit(); err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}

	http.SetCookie(w, &http.Cookie{Name: "witzo_access_token", Value: accessToken, HttpOnly: true, Path: "/", Expires: accessExp})
	http.SetCookie(w, &http.Cookie{Name: "witzo_refresh_token", Value: refreshToken, HttpOnly: true, Path: "/", Expires: refreshExp})
	if firstSignup {
		c.sendWelcomeEmail(email)
	}
	jsonOK(w, map[string]interface{}{"success": true, "message": "Login successful", "accessToken": accessToken, "refreshToken": refreshToken, "user": userResponse(updatedUser, sessionID)})
}

func (c *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	if err := c.RequireCSRF(r); err != nil {
		jsonErr(w, http.StatusForbidden, err.Error())
		return
	}
	refresh := ""
	if c, err := r.Cookie("witzo_refresh_token"); err == nil {
		refresh = c.Value
	}
	if refresh == "" {
		var body struct {
			RefreshToken string `json:"refreshToken"`
		}
		_ = decodeJSON(r, &body)
		refresh = body.RefreshToken
	}
	if refresh == "" {
		jsonErr(w, http.StatusUnauthorized, "refresh token required")
		return
	}
	claims := &TokenClaims{}
	tok, err := jwt.ParseWithClaims(refresh, claims, func(token *jwt.Token) (interface{}, error) { return []byte(c.cfg.JWTRefreshSecret), nil })
	if err != nil || !tok.Valid || claims.Type != "refresh" {
		jsonErr(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	var userEmail string
	err = c.db.QueryRow(`SELECT u.email FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.id=$1 AND s.user_id=$2 AND s.refresh_token=$3 AND s.is_revoked=FALSE AND s.refresh_token_expires_at > CURRENT_TIMESTAMP`, claims.SessionID, claims.UserID, hashToken(refresh)).Scan(&userEmail)
	if err != nil {
		jsonErr(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}
	newAccess, accessExp, err := c.createToken(claims.UserID, userEmail, claims.SessionID, "access", c.cfg.JWTSecret, time.Duration(c.cfg.AccessTokenMinutes)*time.Minute)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "token error")
		return
	}
	newRefresh, refreshExp, err := c.createToken(claims.UserID, userEmail, claims.SessionID, "refresh", c.cfg.JWTRefreshSecret, time.Duration(c.cfg.RefreshTokenDays)*24*time.Hour)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "token error")
		return
	}
	_, err = c.db.Exec(`UPDATE sessions SET access_token=$1, refresh_token=$2, access_token_expires_at=$3, refresh_token_expires_at=$4, updated_at=CURRENT_TIMESTAMP WHERE id=$5`, hashToken(newAccess), hashToken(newRefresh), accessExp, refreshExp, claims.SessionID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "witzo_access_token", Value: newAccess, HttpOnly: true, Path: "/", Expires: accessExp})
	http.SetCookie(w, &http.Cookie{Name: "witzo_refresh_token", Value: newRefresh, HttpOnly: true, Path: "/", Expires: refreshExp})
	jsonOK(w, map[string]interface{}{"success": true, "accessToken": newAccess, "newRefreshToken": newRefresh})
}
func (c *Handler) Logout(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	_, _ = c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE id=$1`, claims.SessionID)
	http.SetCookie(w, &http.Cookie{Name: "witzo_access_token", Value: "", HttpOnly: true, Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "witzo_refresh_token", Value: "", HttpOnly: true, Path: "/", MaxAge: -1})
	jsonOK(w, map[string]interface{}{"success": true, "message": "Logged out"})
}

func (c *Handler) Me(w http.ResponseWriter, _ *http.Request, claims TokenClaims, user UserRecord) {
	jsonOK(w, map[string]interface{}{"success": true, "user": userResponse(user, claims.SessionID)})
}

func (c *Handler) ValidateSession(w http.ResponseWriter, _ *http.Request, claims TokenClaims, user UserRecord) {
	jsonOK(w, map[string]interface{}{"success": true, "valid": true, "user": userResponse(user, claims.SessionID)})
}

func (c *Handler) ProfileStatus(w http.ResponseWriter, _ *http.Request, claims TokenClaims, user UserRecord) {
	jsonOK(w, map[string]interface{}{"success": true, "profileCompleted": user.ProfileCompleted, "plan": user.PlanType, "user": userResponse(user, claims.SessionID)})
}

func (c *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
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
	if err := decodeJSON(r, &body); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid payload")
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
		updated_at=CURRENT_TIMESTAMP WHERE id=$1`, claims.UserID, nullable(body.FullName), nullable(body.CompanyName), nullable(body.PhoneNumber), nullable(body.Country), nullable(body.JobTitle), nullable(body.Industry), nullable(body.CompanyWebsite))
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	row := c.db.QueryRow(`SELECT id,email,is_verified,plan_type,conversations_used,conversations_limit,plan_reset_date,
		login_count,full_name,company_name,phone_number,country,job_title,industry,company_website,
		profile_completed,profile_prompt_required_at,profile_completed_at FROM users WHERE id=$1`, claims.UserID)
	u, err := scanUser(row)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	jsonOK(w, map[string]interface{}{"success": true, "user": userResponse(u, claims.SessionID)})
}

func nullable(v string) interface{} {
	t := strings.TrimSpace(v)
	if t == "" {
		return nil
	}
	return t
}

func (c *Handler) GetSessions(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,user_id,created_at,access_token_expires_at,refresh_token_expires_at,ip_address,user_agent,is_revoked FROM sessions WHERE user_id=$1 ORDER BY created_at DESC`, claims.UserID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
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
		items = append(items, map[string]interface{}{"id": id, "userId": uid, "createdAt": created, "accessTokenExpiresAt": accessExp, "refreshTokenExpiresAt": refreshExp, "ipAddress": nullString(ip), "userAgent": nullString(ua), "isRevoked": revoked})
	}
	jsonOK(w, map[string]interface{}{"success": true, "sessions": items})
}

func (c *Handler) RevokeSession(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	sid := r.PathValue("sessionId")
	_, err := c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE id=$1 AND user_id=$2`, sid, claims.UserID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	jsonOK(w, map[string]interface{}{"success": true})
}

func (c *Handler) RevokeAllOtherSessions(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	_, err := c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE user_id=$1 AND id <> $2`, claims.UserID, claims.SessionID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	jsonOK(w, map[string]interface{}{"success": true})
}

func (c *Handler) LogoutAll(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	_, err := c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE user_id=$1`, claims.UserID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "witzo_access_token", Value: "", HttpOnly: true, Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "witzo_refresh_token", Value: "", HttpOnly: true, Path: "/", MaxAge: -1})
	jsonOK(w, map[string]interface{}{"success": true})
}
