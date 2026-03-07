package controller

import (
	"database/sql"
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
	limits := limitsForPlan(user.PlanType)
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
		"planLimits": map[string]interface{}{
			"scrapedPages":  limits.ScrapedPages,
			"documents":     limits.Documents,
			"documentsMB":   limits.DocumentsMB,
			"conversations": limits.Conversations,
			"chatHistory":   limits.ChatHistory,
			"leads":         limits.Leads,
			"hideBranding":  limits.HideBranding,
			"hasCRM":        limits.HasCRM,
			"hasFollowUp":   limits.HasFollowUp,
			"hasHybrid":     limits.HasHybrid,
			"hasFlows":      limits.HasFlows,
			"hasPersona":    limits.HasPersona,
			"hasNavigation": limits.HasNavigation,
			"roles":         limits.Roles,
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
		IDToken string `json:"idToken"`
		Name    string `json:"name"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.IDToken) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "idToken is required")
		return
	}
	info, err := c.verifyGoogleIDToken(r.Context(), body.IDToken)
	if err != nil {
		c.logRequestWarn(r, "verify google id token failed", err)
		utils.JSONErr(w, http.StatusUnauthorized, "invalid google token")
		return
	}
	email := strings.ToLower(strings.TrimSpace(info.Email))
	name := strings.TrimSpace(info.Name)
	if name == "" {
		name = strings.TrimSpace(body.Name)
	}
	if email == "" {
		utils.JSONErr(w, http.StatusUnauthorized, "google account email is required")
		return
	}
	firstSignup := false
	createdUser := false

	tx, err := c.db.BeginTx(r.Context(), nil)
	if err != nil {
		c.logRequestError(r, "verify google begin transaction failed", err, "email", email)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback()

	row := tx.QueryRow(`SELECT id,email,is_verified,plan_type,conversations_used,conversations_limit,plan_reset_date,
		login_count,full_name,company_name,phone_number,country,job_title,industry,company_website,
		profile_completed,profile_prompt_required_at,profile_completed_at,onboarding_completed_at
		FROM users WHERE email=$1`, email)
	user, queryErr := scanUser(row)
	switch queryErr {
	case nil:
		firstSignup = !user.IsVerified
	default:
		if queryErr != sql.ErrNoRows {
			c.logRequestError(r, "verify google user lookup failed", queryErr, "email", email)
			utils.JSONErr(w, http.StatusInternalServerError, "db error")
			return
		}
		firstSignup = true
		createdUser = true
		if err := tx.QueryRow(`INSERT INTO users (email,full_name,is_verified,profile_completed,last_login,login_count)
			VALUES ($1,$2,TRUE,CASE WHEN COALESCE($2,'')<>'' THEN TRUE ELSE FALSE END,CURRENT_TIMESTAMP,1)
			RETURNING id,email,is_verified,plan_type,conversations_used,conversations_limit,plan_reset_date,
				login_count,full_name,company_name,phone_number,country,job_title,industry,company_website,
				profile_completed,profile_prompt_required_at,profile_completed_at,onboarding_completed_at`,
			email, utils.Nullable(name)).Scan(
			&user.ID, &user.Email, &user.IsVerified, &user.PlanType, &user.ConversationsUsed, &user.ConversationsLimit, &user.PlanResetDate,
			&user.LoginCount, &user.FullName, &user.CompanyName, &user.PhoneNumber, &user.Country, &user.JobTitle, &user.Industry, &user.CompanyWebsite,
			&user.ProfileCompleted, &user.ProfilePromptRequired, &user.ProfileCompletedAt, &user.OnboardingCompletedAt,
		); err != nil {
			c.logRequestError(r, "verify google user create failed", err, "email", email)
			utils.JSONErr(w, http.StatusInternalServerError, "db error")
			return
		}
	}

	if !createdUser {
		if _, err := tx.Exec(`UPDATE users
			SET is_verified=TRUE,
				last_login=CURRENT_TIMESTAMP,
				login_count=login_count+1,
				full_name=CASE WHEN COALESCE(full_name,'')='' THEN $2 ELSE full_name END,
				profile_completed=CASE WHEN profile_completed=FALSE AND COALESCE($2,'')<>'' THEN TRUE ELSE profile_completed END,
				profile_prompt_required_at=CASE
					WHEN (login_count + 1) > 3 AND profile_completed=FALSE THEN COALESCE(profile_prompt_required_at, CURRENT_TIMESTAMP)
					ELSE profile_prompt_required_at
				END,
				updated_at=CURRENT_TIMESTAMP
			WHERE id=$1`,
			user.ID, utils.Nullable(name)); err != nil {
			c.logRequestError(r, "verify google user update failed", err, "email", email, "user_id", user.ID)
			utils.JSONErr(w, http.StatusInternalServerError, "db error")
			return
		}
	}

	var sessionID int64
	if err := tx.QueryRow(`INSERT INTO sessions (user_id,access_token,refresh_token,access_token_expires_at,refresh_token_expires_at,ip_address,user_agent)
		VALUES ($1,'pending','pending',NOW()+INTERVAL '15 minutes',NOW()+INTERVAL '7 days',$2,$3)
		RETURNING id`, user.ID, r.RemoteAddr, r.UserAgent()).Scan(&sessionID); err != nil {
		c.logRequestError(r, "verify google session insert failed", err, "email", email, "user_id", user.ID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	accessToken, accessExp, err := c.createToken(user.ID, email, sessionID, "access", c.cfg.JWTSecret, time.Duration(c.cfg.AccessTokenMinutes)*time.Minute)
	if err != nil {
		c.logRequestError(r, "verify google access token creation failed", err, "email", email, "user_id", user.ID, "session_id", sessionID)
		utils.JSONErr(w, http.StatusInternalServerError, "token error")
		return
	}
	refreshToken, refreshExp, err := c.createToken(user.ID, email, sessionID, "refresh", c.cfg.JWTRefreshSecret, time.Duration(c.cfg.RefreshTokenDays)*24*time.Hour)
	if err != nil {
		c.logRequestError(r, "verify google refresh token creation failed", err, "email", email, "user_id", user.ID, "session_id", sessionID)
		utils.JSONErr(w, http.StatusInternalServerError, "token error")
		return
	}
	if _, err := tx.Exec(`UPDATE sessions
		SET access_token=$1,refresh_token=$2,access_token_expires_at=$3,refresh_token_expires_at=$4
		WHERE id=$5`,
		utils.HashToken(accessToken), utils.HashToken(refreshToken), accessExp, refreshExp, sessionID); err != nil {
		c.logRequestError(r, "verify google session token update failed", err, "email", email, "user_id", user.ID, "session_id", sessionID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	if _, err := tx.Exec(`INSERT INTO widget_keys (user_id, widget_key, widget_name, widget_config, is_active)
		VALUES ($1, $2, 'My Chat Widget', '{}'::jsonb, TRUE)
		ON CONFLICT (user_id) DO NOTHING`, user.ID, generateWidgetKey()); err != nil {
		c.logRequestWarn(r, "verify google ensure widget key failed", err, "email", email, "user_id", user.ID)
	}

	row = tx.QueryRow(`SELECT id,email,is_verified,plan_type,conversations_used,conversations_limit,plan_reset_date,
		login_count,full_name,company_name,phone_number,country,job_title,industry,company_website,
		profile_completed,profile_prompt_required_at,profile_completed_at,onboarding_completed_at
		FROM users WHERE id=$1`, user.ID)
	updatedUser, err := scanUser(row)
	if err != nil {
		c.logRequestError(r, "verify google updated user query failed", err, "email", email, "user_id", user.ID, "session_id", sessionID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := tx.Commit(); err != nil {
		c.logRequestError(r, "verify google transaction commit failed", err, "email", email, "user_id", user.ID, "session_id", sessionID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	c.setAuthCookies(w, accessToken, accessExp, refreshToken, refreshExp)
	if firstSignup {
		c.sendWelcomeEmail(email)
	}
	utils.JSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Login successful",
		"user":    userResponse(updatedUser, sessionID),
	})
}
