package controller

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func (c *Controller) AdminLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if body.Email != c.cfg.AdminEmail || body.Password != c.cfg.AdminPassword {
		jsonErr(w, http.StatusUnauthorized, "invalid admin credentials")
		return
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, TokenClaims{Type: "admin", RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now())}})
	s, _ := token.SignedString([]byte(c.cfg.AdminJWTSecret))
	jsonOK(w, map[string]interface{}{"success": true, "token": s})
}

func (c *Controller) AdminDashboard(w http.ResponseWriter, _ *http.Request) {
	var users, sessions, leads int
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE is_revoked=FALSE`).Scan(&sessions)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM leads`).Scan(&leads)
	jsonOK(w, map[string]interface{}{"success": true, "dashboard": map[string]int{"users": users, "sessions": sessions, "leads": leads}})
}

func (c *Controller) AdminInsights(w http.ResponseWriter, _ *http.Request) {
	rows, err := c.db.Query(`SELECT plan_type,COUNT(*) FROM users GROUP BY plan_type`)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	plans := map[string]int{}
	for rows.Next() {
		var p string
		var c int
		_ = rows.Scan(&p, &c)
		plans[p] = c
	}
	jsonOK(w, map[string]interface{}{"success": true, "insights": map[string]interface{}{"plans": plans}})
}

func (c *Controller) AdminUsers(w http.ResponseWriter, _ *http.Request) {
	rows, err := c.db.Query(`SELECT id,email,is_verified,plan_type,login_count,profile_completed,created_at,last_login FROM users ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
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
		_ = rows.Scan(&id, &email, &verified, &plan, &login, &profile, &created, &lastLogin)
		items = append(items, map[string]interface{}{"id": id, "email": email, "isVerified": verified, "plan_type": plan, "loginCount": login, "profileCompleted": profile, "createdAt": created, "lastLogin": nullTime(lastLogin)})
	}
	jsonOK(w, map[string]interface{}{"success": true, "users": items})
}

func (c *Controller) AdminResetUsage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"userId"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.UserID) == "" {
		jsonErr(w, http.StatusBadRequest, "userId is required")
		return
	}
	_, _ = c.db.Exec(`UPDATE users SET conversations_used=0,plan_reset_date=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, body.UserID)
	jsonOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) AdminForceLogout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"userId"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.UserID) == "" {
		jsonErr(w, http.StatusBadRequest, "userId is required")
		return
	}
	_, _ = c.db.Exec(`UPDATE sessions SET is_revoked=TRUE WHERE user_id=$1`, body.UserID)
	jsonOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) AdminSetPlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"userId"`
		Plan   string `json:"plan"`
	}
	if err := decodeJSON(r, &body); err != nil || body.UserID == "" || body.Plan == "" {
		jsonErr(w, http.StatusBadRequest, "userId and plan are required")
		return
	}
	limit := interface{}(int64(100))
	if body.Plan == "basic" {
		limit = int64(1000)
	} else if body.Plan == "enterprise" {
		limit = nil
	}
	_, _ = c.db.Exec(`UPDATE users SET plan_type=$2,conversations_limit=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, body.UserID, body.Plan, limit)
	jsonOK(w, map[string]interface{}{"success": true})
}
func (c *Controller) PublicWidgetConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("widgetKey")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	cached, err := c.redis.Get(ctx, "widget:"+key).Result()
	if err == nil && cached != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(cached))
		return
	}
	var id int64
	var userID, name string
	var active bool
	var cfgRaw []byte
	err = c.db.QueryRow(`SELECT id,user_id,widget_name,is_active,widget_config FROM widget_keys WHERE widget_key=$1`, key).Scan(&id, &userID, &name, &active, &cfgRaw)
	if err != nil || !active {
		jsonErr(w, http.StatusNotFound, "widget not found")
		return
	}
	_, _ = c.db.Exec(`UPDATE widget_keys SET usage_count=usage_count+1,last_used_at=CURRENT_TIMESTAMP WHERE id=$1`, id)
	resp := map[string]interface{}{"success": true, "widget": map[string]interface{}{"id": id, "userId": userID, "name": name, "widgetKey": key, "settings": json.RawMessage(cfgRaw)}}
	b, _ := json.Marshal(resp)
	_ = c.redis.Set(ctx, "widget:"+key, string(b), 10*time.Minute).Err()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (c *Controller) PublicWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WidgetKey string `json:"widgetKey"`
		Message   string `json:"message"`
		SessionID string `json:"sessionId"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.WidgetKey) == "" {
		jsonErr(w, http.StatusBadRequest, "widgetKey is required")
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		jsonErr(w, http.StatusBadRequest, "message is required")
		return
	}
	var ownerID string
	var widgetID int64
	if err := c.db.QueryRow(`SELECT user_id,id FROM widget_keys WHERE widget_key=$1 AND is_active=TRUE`, body.WidgetKey).Scan(&ownerID, &widgetID); err != nil {
		jsonErr(w, http.StatusNotFound, "widget not found")
		return
	}
	sessionID := strings.TrimSpace(body.SessionID)
	if sessionID == "" {
		sessionID = randomID("sess")
	}
	_, _ = c.db.Exec(`INSERT INTO chat_conversations (id,user_id,status,last_message_preview,last_message_at) VALUES ($1,$2,'active',$3,CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET last_message_preview=EXCLUDED.last_message_preview,last_message_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`, sessionID, ownerID, body.Message)
	matches, _ := c.pineconeQuery(ownerID, body.Message, 3)
	answer := "I'm sorry, I couldn't process your request right now. Please try again."
	if len(matches) > 0 {
		if ai, aiErr := c.openAIAnswerWithContext(body.Message, matches); aiErr == nil && strings.TrimSpace(ai) != "" {
			answer = ai
		}
	} else {
		if ai, aiErr := c.openAIChat(body.Message); aiErr == nil && strings.TrimSpace(ai) != "" {
			answer = ai
		}
	}
	_, _ = c.db.Exec(`INSERT INTO chat_messages (conversation_id,user_id,role,content,metadata) VALUES ($1,$2,'user',$3,'{}'::jsonb),($1,$2,'assistant',$4,'{}'::jsonb)`, sessionID, ownerID, body.Message, answer)
	_, _ = c.db.Exec(`UPDATE chat_conversations SET message_count=message_count+2,last_message_preview=$2,last_message_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, sessionID, body.Message)
	if b, marshalErr := json.Marshal(map[string]interface{}{"widget_key_id": widgetID, "event_type": "message_sent", "event_data": map[string]interface{}{}, "ip": r.RemoteAddr, "user_agent": r.Header.Get("User-Agent"), "referer": r.Referer()}); marshalErr == nil {
		_ = c.redis.RPush(r.Context(), "widget:analytics:buffer", string(b)).Err()
	}
	jsonOK(w, map[string]interface{}{"success": true, "sessionId": sessionID, "response": answer})
}

func (c *Controller) EmbedJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = w.Write([]byte("console.log('Witzo embed loader (Go)');"))
}

func (c *Controller) EmbedForWidget(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/v1/embed/")
	if !strings.HasSuffix(key, ".js") || key == ".js" {
		jsonErr(w, http.StatusNotFound, "embed script not found")
		return
	}
	key = strings.TrimSuffix(key, ".js")
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = w.Write([]byte(fmt.Sprintf("console.log('Witzo widget loaded: %s');", key)))
}

func (c *Controller) PublicContact(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WidgetKey string `json:"widgetKey"`
		SessionID string `json:"sessionId"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		Phone     string `json:"phone"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.WidgetKey) == "" {
		jsonErr(w, http.StatusBadRequest, "widgetKey is required")
		return
	}
	if strings.TrimSpace(body.SessionID) == "" {
		body.SessionID = randomID("sess")
	}
	var widgetID int64
	var ownerID string
	err := c.db.QueryRow(`SELECT id,user_id FROM widget_keys WHERE widget_key=$1 AND is_active=TRUE`, body.WidgetKey).Scan(&widgetID, &ownerID)
	if err != nil {
		jsonErr(w, http.StatusNotFound, "widget not found")
		return
	}
	var leadID string
	err = c.db.QueryRow(`INSERT INTO leads (user_id,widget_key_id,session_id,name,email,phone,status,source_url,ip_address)
		VALUES ($1,$2,$3,$4,$5,$6,'new',$7,$8)
		ON CONFLICT (user_id,session_id) DO UPDATE SET name=COALESCE(EXCLUDED.name,leads.name),email=COALESCE(EXCLUDED.email,leads.email),phone=COALESCE(EXCLUDED.phone,leads.phone),updated_at=CURRENT_TIMESTAMP
		RETURNING id`, ownerID, widgetID, body.SessionID, nullable(body.Name), nullable(body.Email), nullable(body.Phone), nullable(r.Referer()), nullable(r.RemoteAddr)).Scan(&leadID)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.queueWebhookEvent(ownerID, leadID, "lead.created", map[string]interface{}{"leadId": leadID})

	// Auto follow-up email for basic/enterprise plans (fire-and-forget)
	if strings.TrimSpace(body.Email) != "" {
		email := body.Email
		name := body.Name
		lID := leadID
		oID := ownerID
		go func() {
			var planType string
			if err := c.db.QueryRow(`SELECT plan_type FROM users WHERE id=$1`, oID).Scan(&planType); err != nil {
				return
			}
			if planType != "basic" && planType != "enterprise" {
				return
			}
			var followUpSentAt sql.NullTime
			_ = c.db.QueryRow(`SELECT follow_up_sent_at FROM leads WHERE id=$1`, lID).Scan(&followUpSentAt)
			if followUpSentAt.Valid {
				return
			}
			var ownerEmail string
			_ = c.db.QueryRow(`SELECT email FROM users WHERE id=$1`, oID).Scan(&ownerEmail)
			c.sendFollowUpEmail(email, name, ownerEmail)
			_, _ = c.db.Exec(`UPDATE leads SET follow_up_sent_at=CURRENT_TIMESTAMP WHERE id=$1`, lID)
		}()
	}

	jsonOK(w, map[string]interface{}{"success": true, "lead": map[string]interface{}{"id": leadID}})
}

func (c *Controller) PublicRating(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WidgetKey string `json:"widgetKey"`
		SessionID string `json:"sessionId"`
		Rating    string `json:"rating"`
	}
	if err := decodeJSON(r, &body); err != nil || body.WidgetKey == "" || body.SessionID == "" {
		jsonErr(w, http.StatusBadRequest, "widgetKey and sessionId are required")
		return
	}
	if body.Rating != "up" && body.Rating != "down" {
		if body.Rating == "??" || strings.EqualFold(body.Rating, "like") {
			body.Rating = "up"
		} else {
			body.Rating = "down"
		}
	}
	var ownerID string
	var widgetID int64
	err := c.db.QueryRow(`SELECT user_id,id FROM widget_keys WHERE widget_key=$1`, body.WidgetKey).Scan(&ownerID, &widgetID)
	if err != nil {
		jsonErr(w, http.StatusNotFound, "widget not found")
		return
	}
	_, err = c.db.Exec(`INSERT INTO chat_ratings (user_id,session_id,widget_key_id,rating) VALUES ($1,$2,$3,$4) ON CONFLICT (user_id,session_id) DO UPDATE SET rating=EXCLUDED.rating`, ownerID, body.SessionID, widgetID, body.Rating)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "db error")
		return
	}
	jsonOK(w, map[string]interface{}{"success": true, "rating": body.Rating})
}

func (c *Controller) Health(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]interface{}{"status": "healthy", "timestamp": time.Now().UTC(), "uptime": time.Since(startedAt).Seconds()})
}

func (c *Controller) HealthDetailed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	dbStatus := "healthy"
	if err := c.db.PingContext(ctx); err != nil {
		dbStatus = "unhealthy"
	}
	redisStatus := "healthy"
	if err := c.redis.Ping(ctx).Err(); err != nil {
		redisStatus = "unhealthy"
	}
	status := "healthy"
	if dbStatus != "healthy" || redisStatus != "healthy" {
		status = "degraded"
	}
	code := http.StatusOK
	if status != "healthy" {
		code = http.StatusServiceUnavailable
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": status, "dependencies": map[string]string{"database": dbStatus, "redis": redisStatus}, "timestamp": time.Now().UTC()})
}

func (c *Controller) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := c.db.PingContext(ctx); err != nil {
		jsonErr(w, http.StatusServiceUnavailable, "not ready")
		return
	}
	jsonOK(w, map[string]interface{}{"status": "ready", "timestamp": time.Now().UTC()})
}

func (c *Controller) Live(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]interface{}{"status": "alive", "timestamp": time.Now().UTC()})
}

func (c *Controller) Metrics(w http.ResponseWriter, _ *http.Request) {
	var users, sessions int
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE is_revoked=FALSE`).Scan(&sessions)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(fmt.Sprintf("witzo_users_total %d\nwitzo_sessions_total %d\n", users, sessions)))
}
