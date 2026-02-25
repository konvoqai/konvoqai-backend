package controller

import (
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

func (c *Controller) Overview(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	var chats, leads, sources, widgetViews, widgetMessages, ratings int
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM chat_conversations WHERE user_id=$1 AND is_deleted=FALSE`, claims.UserID).Scan(&chats)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM leads WHERE user_id=$1`, claims.UserID).Scan(&leads)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&sources)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM widget_analytics wa JOIN widget_keys wk ON wa.widget_key_id=wk.id WHERE wk.user_id=$1 AND wa.event_type='widget_loaded'`, claims.UserID).Scan(&widgetViews)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM widget_analytics wa JOIN widget_keys wk ON wa.widget_key_id=wk.id WHERE wk.user_id=$1 AND wa.event_type='message_sent'`, claims.UserID).Scan(&widgetMessages)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM chat_ratings WHERE user_id=$1`, claims.UserID).Scan(&ratings)
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
	_, _ = c.db.Exec(`INSERT INTO users (email,is_verified,full_name,profile_completed) VALUES ($1,TRUE,$2,CASE WHEN COALESCE($2,'')<>'' THEN TRUE ELSE FALSE END) ON CONFLICT (email) DO UPDATE SET is_verified=TRUE,last_login=CURRENT_TIMESTAMP`,
		email, utils.Nullable(body.Name))
	row := c.db.QueryRow(`SELECT id,email,is_verified,plan_type,conversations_used,conversations_limit,plan_reset_date,
		login_count,full_name,company_name,phone_number,country,job_title,industry,company_website,
		profile_completed,profile_prompt_required_at,profile_completed_at FROM users WHERE email=$1`, email)
	user, err := scanUser(row)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	var sessionID int64
	_ = c.db.QueryRow(`INSERT INTO sessions (user_id,access_token,refresh_token,access_token_expires_at,refresh_token_expires_at,ip_address,user_agent) VALUES ($1,'pending','pending',NOW()+INTERVAL '15 minutes',NOW()+INTERVAL '7 days',$2,$3) RETURNING id`,
		user.ID, r.RemoteAddr, r.UserAgent()).Scan(&sessionID)
	at, ae, _ := c.createToken(user.ID, user.Email, sessionID, "access", c.cfg.JWTSecret, time.Duration(c.cfg.AccessTokenMinutes)*time.Minute)
	rt, re, _ := c.createToken(user.ID, user.Email, sessionID, "refresh", c.cfg.JWTRefreshSecret, time.Duration(c.cfg.RefreshTokenDays)*24*time.Hour)
	_, _ = c.db.Exec(`UPDATE sessions SET access_token=$1,refresh_token=$2,access_token_expires_at=$3,refresh_token_expires_at=$4 WHERE id=$5`,
		utils.HashToken(at), utils.HashToken(rt), ae, re, sessionID)
	http.SetCookie(w, &http.Cookie{Name: "witzo_access_token", Value: at, HttpOnly: true, Path: "/", Expires: ae})
	http.SetCookie(w, &http.Cookie{Name: "witzo_refresh_token", Value: rt, HttpOnly: true, Path: "/", Expires: re})
	utils.JSONOK(w, map[string]interface{}{"success": true, "user": userResponse(user, sessionID), "accessToken": at, "refreshToken": rt})
}
