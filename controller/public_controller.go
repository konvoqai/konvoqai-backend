package controller

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"konvoq-backend/utils"
)

func (c *Controller) PublicWidgetConfig(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "widgetKey")
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
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			c.logRequestError(r, "public widget config query failed", err, "widget_key", key)
		}
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	if _, err := c.db.Exec(`UPDATE widget_keys SET usage_count=usage_count+1,last_used_at=CURRENT_TIMESTAMP WHERE id=$1`, id); err != nil {
		c.logRequestWarn(r, "public widget usage counter update failed", err, "widget_key_id", id)
	}
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
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.WidgetKey) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "widgetKey is required")
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "message is required")
		return
	}
	var ownerID string
	var widgetID int64
	if err := c.db.QueryRow(`SELECT user_id,id FROM widget_keys WHERE widget_key=$1 AND is_active=TRUE`, body.WidgetKey).Scan(&ownerID, &widgetID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			c.logRequestError(r, "public webhook widget lookup failed", err, "widget_key", body.WidgetKey)
		}
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	sessionID := strings.TrimSpace(body.SessionID)
	if sessionID == "" {
		sessionID = utils.RandomID("sess")
	}
	if _, err := c.db.Exec(`INSERT INTO chat_conversations (id,user_id,status,last_message_preview,last_message_at) VALUES ($1,$2,'active',$3,CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET last_message_preview=EXCLUDED.last_message_preview,last_message_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`, sessionID, ownerID, body.Message); err != nil {
		c.logRequestWarn(r, "public webhook conversation upsert failed", err, "widget_key", body.WidgetKey, "session_id", sessionID)
	}
	matches, matchErr := c.pineconeQuery(ownerID, body.Message, 3)
	if matchErr != nil {
		c.logRequestWarn(r, "public webhook context lookup failed", matchErr, "widget_key", body.WidgetKey, "session_id", sessionID)
	}
	answer := "I'm sorry, I couldn't process your request right now. Please try again."
	if len(matches) > 0 {
		if ai, aiErr := c.openAIAnswerWithContext(body.Message, matches); aiErr == nil && strings.TrimSpace(ai) != "" {
			answer = ai
		} else if aiErr != nil {
			c.logRequestWarn(r, "public webhook response generation with context failed", aiErr, "widget_key", body.WidgetKey, "session_id", sessionID)
		}
	} else {
		if ai, aiErr := c.openAIChat(body.Message); aiErr == nil && strings.TrimSpace(ai) != "" {
			answer = ai
		} else if aiErr != nil {
			c.logRequestWarn(r, "public webhook response generation failed", aiErr, "widget_key", body.WidgetKey, "session_id", sessionID)
		}
	}
	if _, err := c.db.Exec(`INSERT INTO chat_messages (conversation_id,user_id,role,content,metadata) VALUES ($1,$2,'user',$3,'{}'::jsonb),($1,$2,'assistant',$4,'{}'::jsonb)`, sessionID, ownerID, body.Message, answer); err != nil {
		c.logRequestWarn(r, "public webhook message insert failed", err, "widget_key", body.WidgetKey, "session_id", sessionID)
	}
	if _, err := c.db.Exec(`UPDATE chat_conversations SET message_count=message_count+2,last_message_preview=$2,last_message_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, sessionID, body.Message); err != nil {
		c.logRequestWarn(r, "public webhook conversation update failed", err, "widget_key", body.WidgetKey, "session_id", sessionID)
	}
	if b, marshalErr := json.Marshal(map[string]interface{}{
		"widget_key_id": widgetID, "event_type": "message_sent", "event_data": map[string]interface{}{},
		"ip": r.RemoteAddr, "user_agent": r.Header.Get("User-Agent"), "referer": r.Referer(),
	}); marshalErr == nil {
		_ = c.redis.RPush(r.Context(), "widget:analytics:buffer", string(b)).Err()
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "sessionId": sessionID, "response": answer})
}

func (c *Controller) EmbedJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = w.Write([]byte("console.log('Witzo embed loader (Go)');"))
}

func (c *Controller) EmbedForWidget(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/v1/embed/")
	if !strings.HasSuffix(key, ".js") || key == ".js" {
		utils.JSONErr(w, http.StatusNotFound, "embed script not found")
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
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.WidgetKey) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "widgetKey is required")
		return
	}
	if strings.TrimSpace(body.SessionID) == "" {
		body.SessionID = utils.RandomID("sess")
	}
	var widgetID int64
	var ownerID string
	err := c.db.QueryRow(`SELECT id,user_id FROM widget_keys WHERE widget_key=$1 AND is_active=TRUE`, body.WidgetKey).Scan(&widgetID, &ownerID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			c.logRequestError(r, "public contact widget lookup failed", err, "widget_key", body.WidgetKey)
		}
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	var leadID string
	err = c.db.QueryRow(`INSERT INTO leads (user_id,widget_key_id,session_id,name,email,phone,status,source_url,ip_address)
		VALUES ($1,$2,$3,$4,$5,$6,'new',$7,$8)
		ON CONFLICT (user_id,session_id) DO UPDATE SET name=COALESCE(EXCLUDED.name,leads.name),email=COALESCE(EXCLUDED.email,leads.email),phone=COALESCE(EXCLUDED.phone,leads.phone),updated_at=CURRENT_TIMESTAMP
		RETURNING id`,
		ownerID, widgetID, body.SessionID, utils.Nullable(body.Name), utils.Nullable(body.Email), utils.Nullable(body.Phone),
		utils.Nullable(r.Referer()), utils.Nullable(r.RemoteAddr)).Scan(&leadID)
	if err != nil {
		c.logRequestError(r, "public contact lead upsert failed", err, "widget_key", body.WidgetKey, "owner_id", ownerID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	c.queueWebhookEvent(ownerID, leadID, "lead.created", map[string]interface{}{"leadId": leadID})

	if strings.TrimSpace(body.Email) != "" {
		email := body.Email
		name := body.Name
		lID := leadID
		oID := ownerID
		go func() {
			var planType string
			if err := c.db.QueryRow(`SELECT plan_type FROM users WHERE id=$1`, oID).Scan(&planType); err != nil {
				c.logger.Warn("public contact follow-up plan lookup failed", "owner_id", oID, "lead_id", lID, "error", err)
				return
			}
			if planType != "basic" && planType != "enterprise" {
				return
			}
			var followUpSentAt sql.NullTime
			if err := c.db.QueryRow(`SELECT follow_up_sent_at FROM leads WHERE id=$1`, lID).Scan(&followUpSentAt); err != nil {
				c.logger.Warn("public contact follow-up state lookup failed", "lead_id", lID, "error", err)
				return
			}
			if followUpSentAt.Valid {
				return
			}
			var ownerEmail string
			if err := c.db.QueryRow(`SELECT email FROM users WHERE id=$1`, oID).Scan(&ownerEmail); err != nil {
				c.logger.Warn("public contact owner email lookup failed", "owner_id", oID, "lead_id", lID, "error", err)
				return
			}
			c.sendFollowUpEmail(email, name, ownerEmail)
			if _, err := c.db.Exec(`UPDATE leads SET follow_up_sent_at=CURRENT_TIMESTAMP WHERE id=$1`, lID); err != nil {
				c.logger.Warn("public contact follow-up timestamp update failed", "lead_id", lID, "error", err)
			}
		}()
	}

	utils.JSONOK(w, map[string]interface{}{"success": true, "lead": map[string]interface{}{"id": leadID}})
}

func (c *Controller) PublicRating(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WidgetKey string `json:"widgetKey"`
		SessionID string `json:"sessionId"`
		Rating    string `json:"rating"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || body.WidgetKey == "" || body.SessionID == "" {
		utils.JSONErr(w, http.StatusBadRequest, "widgetKey and sessionId are required")
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
		if !errors.Is(err, sql.ErrNoRows) {
			c.logRequestError(r, "public rating widget lookup failed", err, "widget_key", body.WidgetKey, "session_id", body.SessionID)
		}
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	_, err = c.db.Exec(`INSERT INTO chat_ratings (user_id,session_id,widget_key_id,rating) VALUES ($1,$2,$3,$4) ON CONFLICT (user_id,session_id) DO UPDATE SET rating=EXCLUDED.rating`,
		ownerID, body.SessionID, widgetID, body.Rating)
	if err != nil {
		c.logRequestError(r, "public rating upsert failed", err, "owner_id", ownerID, "session_id", body.SessionID, "widget_key_id", widgetID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "rating": body.Rating})
}
