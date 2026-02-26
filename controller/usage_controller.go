package controller

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"konvoq-backend/utils"
)

func (c *Controller) GetUsage(w http.ResponseWriter, _ *http.Request, _ TokenClaims, user UserRecord) {
	var remaining interface{}
	var atLimit bool
	if user.ConversationsLimit.Valid {
		left := int(user.ConversationsLimit.Int64) - user.ConversationsUsed
		if left < 0 {
			left = 0
		}
		remaining = left
		atLimit = user.ConversationsUsed >= int(user.ConversationsLimit.Int64)
	}
	utils.JSONOK(w, map[string]interface{}{
		"success": true,
		"usage": map[string]interface{}{
			"planType":               user.PlanType,
			"conversationsUsed":      user.ConversationsUsed,
			"conversationsLimit":     utils.NullableInt64(user.ConversationsLimit),
			"conversationsRemaining": remaining,
			"resetDate":              user.PlanResetDate.AddDate(0, 1, 0),
			"isAtLimit":              atLimit,
		},
	})
}

func (c *Controller) Overview(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var chats, leads, sources, widgetViews, widgetMessages, ratings int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM chat_conversations WHERE user_id=$1 AND is_deleted=FALSE`, claims.UserID).Scan(&chats); err != nil {
		c.logRequestWarn(r, "overview chat count failed", err, "user_id", claims.UserID)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM leads WHERE user_id=$1`, claims.UserID).Scan(&leads); err != nil {
		c.logRequestWarn(r, "overview leads count failed", err, "user_id", claims.UserID)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&sources); err != nil {
		c.logRequestWarn(r, "overview sources count failed", err, "user_id", claims.UserID)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM widget_analytics wa JOIN widget_keys wk ON wa.widget_key_id=wk.id WHERE wk.user_id=$1 AND wa.event_type='widget_loaded'`, claims.UserID).Scan(&widgetViews); err != nil {
		c.logRequestWarn(r, "overview widget views count failed", err, "user_id", claims.UserID)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM widget_analytics wa JOIN widget_keys wk ON wa.widget_key_id=wk.id WHERE wk.user_id=$1 AND wa.event_type='message_sent'`, claims.UserID).Scan(&widgetMessages); err != nil {
		c.logRequestWarn(r, "overview widget messages count failed", err, "user_id", claims.UserID)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM chat_ratings WHERE user_id=$1`, claims.UserID).Scan(&ratings); err != nil {
		c.logRequestWarn(r, "overview ratings count failed", err, "user_id", claims.UserID)
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "analytics": map[string]interface{}{
		"chatSessions":   chats,
		"leads":          leads,
		"sources":        sources,
		"widgetViews":    widgetViews,
		"widgetMessages": widgetMessages,
		"totalRatings":   ratings,
	}})
}

func (c *Controller) VerifyGoogle(w http.ResponseWriter, r *http.Request) {
	if err := c.RequireCSRF(r); err != nil {
		utils.JSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	var body struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Email) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "email is required")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	name := strings.TrimSpace(body.Name)

	tx, err := c.db.BeginTx(r.Context(), nil)
	if err != nil {
		c.logRequestError(r, "verify google begin transaction failed", err, "email", email)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	var userID string
	err = tx.QueryRow(`SELECT id FROM users WHERE email=$1`, email).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRow(`INSERT INTO users (email,full_name,profile_completed) VALUES ($1,$2,CASE WHEN COALESCE($2,'')<>'' THEN TRUE ELSE FALSE END) RETURNING id`,
			email, utils.Nullable(name)).Scan(&userID)
	}
	if err != nil {
		c.logRequestError(r, "verify google user lookup/create failed", err, "email", email)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if name != "" {
		if _, err := tx.Exec(`UPDATE users SET full_name=CASE WHEN COALESCE(full_name,'')='' THEN $2 ELSE full_name END, profile_completed=CASE WHEN profile_completed=FALSE AND COALESCE($2,'')<>'' THEN TRUE ELSE profile_completed END WHERE id=$1`,
			userID, name); err != nil {
			c.logRequestError(r, "verify google user update failed", err, "email", email, "user_id", userID)
			utils.JSONErr(w, http.StatusInternalServerError, "db error")
			return
		}
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
	if _, err := tx.Exec(`INSERT INTO verification_codes (user_id,code,expires_at) VALUES ($1,$2,$3)`, userID, code, expires); err != nil {
		c.logRequestError(r, "verify google code insert failed", err, "user_id", userID, "email", email)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := tx.Commit(); err != nil {
		c.logRequestError(r, "verify google transaction commit failed", err, "user_id", userID, "email", email)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.sendVerificationEmail(email, code)
	utils.JSONOK(w, map[string]interface{}{
		"success":   true,
		"message":   "Verification code sent",
		"email":     email,
		"expiresIn": c.cfg.VerifyCodeMinutes * 60,
	})
}
