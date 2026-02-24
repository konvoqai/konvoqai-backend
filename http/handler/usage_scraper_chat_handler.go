package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"
)

func (c *Handler) GetUsage(w http.ResponseWriter, _ *http.Request, _ TokenClaims, user UserRecord) {
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
	jsonOK(w, map[string]interface{}{
		"success": true,
		"usage": map[string]interface{}{
			"planType":               user.PlanType,
			"conversationsUsed":      user.ConversationsUsed,
			"conversationsLimit":     nullableInt(user.ConversationsLimit),
			"conversationsRemaining": remaining,
			"resetDate":              user.PlanResetDate.AddDate(0, 1, 0),
			"isAtLimit":              atLimit,
		},
	})
}

func nullableInt(v sql.NullInt64) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func (c *Handler) Overview(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	var chats, leads, sources, widgetViews, widgetMessages, ratings int
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM chat_conversations WHERE user_id=$1 AND is_deleted=FALSE`, claims.UserID).Scan(&chats)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM leads WHERE user_id=$1`, claims.UserID).Scan(&leads)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&sources)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM widget_analytics wa JOIN widget_keys wk ON wa.widget_key_id=wk.id WHERE wk.user_id=$1 AND wa.event_type='widget_loaded'`, claims.UserID).Scan(&widgetViews)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM widget_analytics wa JOIN widget_keys wk ON wa.widget_key_id=wk.id WHERE wk.user_id=$1 AND wa.event_type='message_sent'`, claims.UserID).Scan(&widgetMessages)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM chat_ratings WHERE user_id=$1`, claims.UserID).Scan(&ratings)
	jsonOK(w, map[string]interface{}{"success": true, "analytics": map[string]interface{}{
		"chatSessions":   chats,
		"leads":          leads,
		"sources":        sources,
		"widgetViews":    widgetViews,
		"widgetMessages": widgetMessages,
		"totalRatings":   ratings,
	}})
}

func (c *Handler) Scrape(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.URL) == "" {
		jsonErr(w, http.StatusBadRequest, "url is required")
		return
	}
	_, err := c.db.Exec(`INSERT INTO scraper_sources (user_id,source_url,source_title) VALUES ($1,$2,$2) ON CONFLICT (user_id,source_url) DO UPDATE SET source_title=EXCLUDED.source_title`, claims.UserID, strings.TrimSpace(body.URL))
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	go func(userID, sourceURL string) {
		text, err := c.extractTextFromURL(sourceURL)
		if err != nil || strings.TrimSpace(text) == "" {
			return
		}
		_ = c.pineconeUpsert(userID, sourceURL, text)
	}(claims.UserID, strings.TrimSpace(body.URL))
	jsonOK(w, map[string]interface{}{"success": true, "message": "scrape queued", "url": body.URL})
}

func (c *Handler) QueryDocuments(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body struct {
		Query string `json:"query"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Query) == "" {
		jsonErr(w, http.StatusBadRequest, "query is required")
		return
	}
	var sources, docs int
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&sources)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM documents WHERE user_id=$1`, claims.UserID).Scan(&docs)
	matches, _ := c.pineconeQuery(claims.UserID, body.Query, 3)
	answer := "I found relevant documents but could not generate an answer right now."
	if ai, err := c.openAIAnswerWithContext(body.Query, matches); err == nil && strings.TrimSpace(ai) != "" {
		answer = ai
	}
	jsonOK(w, map[string]interface{}{"success": true, "answer": answer, "documentsSearched": sources + docs, "matches": matches})
}

func (c *Handler) DeleteSource(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	url := r.URL.Query().Get("url")
	if url == "" {
		var body struct {
			URL string `json:"url"`
		}
		_ = decodeJSON(r, &body)
		url = body.URL
	}
	if strings.TrimSpace(url) == "" {
		jsonErr(w, http.StatusBadRequest, "url is required")
		return
	}
	_, err := c.db.Exec(`DELETE FROM scraper_sources WHERE user_id=$1 AND source_url=$2`, claims.UserID, strings.TrimSpace(url))
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	jsonOK(w, map[string]interface{}{"success": true})
}

func (c *Handler) DeleteAllSources(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	_, _ = c.db.Exec(`DELETE FROM scraper_sources WHERE user_id=$1`, claims.UserID)
	_, _ = c.db.Exec(`DELETE FROM documents WHERE user_id=$1`, claims.UserID)
	jsonOK(w, map[string]interface{}{"success": true})
}

func (c *Handler) SourceStats(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	var sources, docs int
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&sources)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM documents WHERE user_id=$1`, claims.UserID).Scan(&docs)
	jsonOK(w, map[string]interface{}{"success": true, "stats": map[string]int{"sources": sources, "documents": docs}})
}

func (c *Handler) GetSources(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,source_url,source_title,created_at FROM scraper_sources WHERE user_id=$1 ORDER BY created_at DESC`, claims.UserID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id, url string
		var title sql.NullString
		var created time.Time
		_ = rows.Scan(&id, &url, &title, &created)
		items = append(items, map[string]interface{}{"id": id, "url": url, "title": nullString(title), "createdAt": created})
	}
	jsonOK(w, map[string]interface{}{"success": true, "sources": items})
}
func (c *Handler) Chat(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body struct {
		Message   string `json:"message"`
		SessionID string `json:"sessionId"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Message) == "" {
		jsonErr(w, http.StatusBadRequest, "message is required")
		return
	}
	var used int
	var limit sql.NullInt64
	err := c.db.QueryRow(`UPDATE users SET conversations_used=conversations_used+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND (conversations_limit IS NULL OR conversations_used < conversations_limit) RETURNING conversations_used,conversations_limit`, claims.UserID).Scan(&used, &limit)
	if err == sql.ErrNoRows {
		jsonErr(w, http.StatusPaymentRequired, "conversation limit reached")
		return
	}
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	convID := strings.TrimSpace(body.SessionID)
	if convID == "" {
		err = c.db.QueryRow(`INSERT INTO chat_conversations (user_id,status,last_message_preview,last_message_at) VALUES ($1,'active',$2,CURRENT_TIMESTAMP) RETURNING id`, claims.UserID, body.Message).Scan(&convID)
	} else {
		_, err = c.db.Exec(`INSERT INTO chat_conversations (id,user_id,status,last_message_preview,last_message_at) VALUES ($1,$2,'active',$3,CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET last_message_preview=EXCLUDED.last_message_preview,last_message_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`, convID, claims.UserID, body.Message)
	}
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "failed to create conversation")
		return
	}
	answer := "I couldn't generate a response right now. Please try again."
	if ragMatches, _ := c.pineconeQuery(claims.UserID, body.Message, 3); len(ragMatches) > 0 {
		if ai, aiErr := c.openAIAnswerWithContext(body.Message, ragMatches); aiErr == nil && strings.TrimSpace(ai) != "" {
			answer = ai
		}
	} else if ai, aiErr := c.openAIChat(body.Message); aiErr == nil && strings.TrimSpace(ai) != "" {
		answer = ai
	}
	_, _ = c.db.Exec(`INSERT INTO chat_messages (conversation_id,user_id,role,content,metadata) VALUES ($1,$2,'user',$3,'{}'::jsonb),($1,$2,'assistant',$4,'{}'::jsonb)`, convID, claims.UserID, body.Message, answer)
	_, _ = c.db.Exec(`UPDATE chat_conversations SET message_count=message_count+2,last_message_preview=$2,last_message_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, convID, body.Message)
	jsonOK(w, map[string]interface{}{"success": true, "sessionId": convID, "response": answer, "usage": map[string]interface{}{"conversationsUsed": used, "conversationsLimit": nullableInt(limit)}})
}

func (c *Handler) ChatSessions(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,status,message_count,last_message_preview,last_message_at,created_at,updated_at FROM chat_conversations WHERE user_id=$1 AND is_deleted=FALSE ORDER BY last_message_at DESC`, claims.UserID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id, status string
		var cnt int
		var preview sql.NullString
		var lm, c, u time.Time
		_ = rows.Scan(&id, &status, &cnt, &preview, &lm, &c, &u)
		items = append(items, map[string]interface{}{"id": id, "status": status, "messageCount": cnt, "lastMessagePreview": nullString(preview), "lastMessageAt": lm, "createdAt": c, "updatedAt": u})
	}
	jsonOK(w, map[string]interface{}{"success": true, "sessions": items})
}

func (c *Handler) ChatSession(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	sid := r.PathValue("sessionId")
	var exists bool
	_ = c.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM chat_conversations WHERE id=$1 AND user_id=$2 AND is_deleted=FALSE)`, sid, claims.UserID).Scan(&exists)
	if !exists {
		jsonErr(w, http.StatusNotFound, "chat session not found")
		return
	}
	rows, err := c.db.Query(`SELECT role,content,created_at FROM chat_messages WHERE conversation_id=$1 ORDER BY created_at ASC,id ASC`, sid)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	msgs := []map[string]interface{}{}
	for rows.Next() {
		var role, content string
		var created time.Time
		_ = rows.Scan(&role, &content, &created)
		msgs = append(msgs, map[string]interface{}{"role": role, "content": content, "createdAt": created})
	}
	jsonOK(w, map[string]interface{}{"success": true, "session": map[string]interface{}{"id": sid, "messages": msgs}})
}

func (c *Handler) ClearChatSession(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	sid := r.PathValue("sessionId")
	_, _ = c.db.Exec(`UPDATE chat_conversations SET is_deleted=TRUE,status='closed',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND user_id=$2`, sid, claims.UserID)
	jsonOK(w, map[string]interface{}{"success": true})
}

func (c *Handler) ClearUserSessions(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	_, _ = c.db.Exec(`UPDATE chat_conversations SET is_deleted=TRUE,status='closed',updated_at=CURRENT_TIMESTAMP WHERE user_id=$1`, claims.UserID)
	jsonOK(w, map[string]interface{}{"success": true})
}

func (c *Handler) VerifyGoogle(w http.ResponseWriter, r *http.Request) {
	if err := c.RequireCSRF(r); err != nil {
		jsonErr(w, http.StatusForbidden, err.Error())
		return
	}
	var body struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Email) == "" {
		jsonErr(w, http.StatusBadRequest, "email is required")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	_, _ = c.db.Exec(`INSERT INTO users (email,is_verified,full_name,profile_completed) VALUES ($1,TRUE,$2,CASE WHEN COALESCE($2,'')<>'' THEN TRUE ELSE FALSE END) ON CONFLICT (email) DO UPDATE SET is_verified=TRUE,last_login=CURRENT_TIMESTAMP`, email, nullable(body.Name))
	var user UserRecord
	row := c.db.QueryRow(`SELECT id,email,is_verified,plan_type,conversations_used,conversations_limit,plan_reset_date,
		login_count,full_name,company_name,phone_number,country,job_title,industry,company_website,
		profile_completed,profile_prompt_required_at,profile_completed_at FROM users WHERE email=$1`, email)
	u, err := scanUser(row)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	user = u
	var sessionID int64
	_ = c.db.QueryRow(`INSERT INTO sessions (user_id,access_token,refresh_token,access_token_expires_at,refresh_token_expires_at,ip_address,user_agent) VALUES ($1,'pending','pending',NOW()+INTERVAL '15 minutes',NOW()+INTERVAL '7 days',$2,$3) RETURNING id`, user.ID, r.RemoteAddr, r.UserAgent()).Scan(&sessionID)
	at, ae, _ := c.createToken(user.ID, user.Email, sessionID, "access", c.cfg.JWTSecret, time.Duration(c.cfg.AccessTokenMinutes)*time.Minute)
	rt, re, _ := c.createToken(user.ID, user.Email, sessionID, "refresh", c.cfg.JWTRefreshSecret, time.Duration(c.cfg.RefreshTokenDays)*24*time.Hour)
	_, _ = c.db.Exec(`UPDATE sessions SET access_token=$1,refresh_token=$2,access_token_expires_at=$3,refresh_token_expires_at=$4 WHERE id=$5`, hashToken(at), hashToken(rt), ae, re, sessionID)
	http.SetCookie(w, &http.Cookie{Name: "witzo_access_token", Value: at, HttpOnly: true, Path: "/", Expires: ae})
	http.SetCookie(w, &http.Cookie{Name: "witzo_refresh_token", Value: rt, HttpOnly: true, Path: "/", Expires: re})
	jsonOK(w, map[string]interface{}{"success": true, "user": userResponse(user, sessionID), "accessToken": at, "refreshToken": rt})
}
