package controller

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"konvoq-backend/utils"
)

var publicSessionUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func (c *Controller) PublicWidgetConfig(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(chi.URLParam(r, "widgetKey"))
	if key == "" {
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	cached, err := c.redis.Get(ctx, "widget:"+key).Result()
	if err == nil && cached != "" {
		var cachedPayload struct {
			Response       json.RawMessage `json:"response"`
			WidgetID       int64           `json:"widgetId"`
			AllowedDomains []string        `json:"allowedDomains"`
		}
		if unmarshalErr := json.Unmarshal([]byte(cached), &cachedPayload); unmarshalErr == nil {
			if !isWidgetRequestAllowed(cachedPayload.AllowedDomains, r) {
				utils.JSONErr(w, http.StatusForbidden, "widget access denied for this domain")
				return
			}
			if cachedPayload.WidgetID > 0 {
				c.queueWidgetAnalytics(ctx, cachedPayload.WidgetID, "widget_loaded", map[string]interface{}{}, r)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cachedPayload.Response)
			return
		}
	}
	var id int64
	var name string
	var active bool
	var cfgRaw []byte
	var domainsRaw string
	err = c.db.QueryRow(`SELECT id,widget_name,is_active,widget_config,COALESCE(to_json(allowed_domains),'[]'::json)::text FROM widget_keys WHERE widget_key=$1`,
		key).Scan(&id, &name, &active, &cfgRaw, &domainsRaw)
	if err != nil || !active {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			c.logRequestError(r, "public widget config query failed", err, "widget_key", key)
		}
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	allowedDomains := parseAllowedDomainsJSON(domainsRaw)
	if c.cfg.IsProduction && len(allowedDomains) == 0 {
		utils.JSONErr(w, http.StatusForbidden, "widget allowed domains are not configured")
		return
	}
	if !isWidgetRequestAllowed(allowedDomains, r) {
		utils.JSONErr(w, http.StatusForbidden, "widget access denied for this domain")
		return
	}
	if _, err := c.db.Exec(`UPDATE widget_keys SET usage_count=usage_count+1,last_used_at=CURRENT_TIMESTAMP WHERE id=$1`, id); err != nil {
		c.logRequestWarn(r, "public widget usage counter update failed", err, "widget_key_id", id)
	}
	c.queueWidgetAnalytics(ctx, id, "widget_loaded", map[string]interface{}{}, r)
	resp := map[string]interface{}{"success": true, "widget": map[string]interface{}{"id": id, "name": name, "widgetKey": key, "settings": json.RawMessage(cfgRaw)}}
	responseBytes, _ := json.Marshal(resp)
	cachedBytes, _ := json.Marshal(map[string]interface{}{
		"response":       json.RawMessage(responseBytes),
		"widgetId":       id,
		"allowedDomains": allowedDomains,
	})
	_ = c.redis.Set(ctx, "widget:"+key, string(cachedBytes), 10*time.Minute).Err()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseBytes)
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
	body.WidgetKey = strings.TrimSpace(body.WidgetKey)
	var ownerID string
	var widgetID int64
	var domainsRaw string
	if err := c.db.QueryRow(`SELECT user_id,id,COALESCE(to_json(allowed_domains),'[]'::json)::text FROM widget_keys WHERE widget_key=$1 AND is_active=TRUE`, body.WidgetKey).Scan(&ownerID, &widgetID, &domainsRaw); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			c.logRequestError(r, "public webhook widget lookup failed", err, "widget_key", body.WidgetKey)
		}
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	allowedDomains := parseAllowedDomainsJSON(domainsRaw)
	if c.cfg.IsProduction && len(allowedDomains) == 0 {
		utils.JSONErr(w, http.StatusForbidden, "widget allowed domains are not configured")
		return
	}
	if !isWidgetRequestAllowed(allowedDomains, r) {
		utils.JSONErr(w, http.StatusForbidden, "widget access denied for this domain")
		return
	}
	sessionID := strings.TrimSpace(body.SessionID)
	if sessionID == "" {
		if err := c.db.QueryRow(`INSERT INTO chat_conversations (user_id,widget_key_id,status,last_message_preview,last_message_at) VALUES ($1,$2,'active',$3,CURRENT_TIMESTAMP) RETURNING id`,
			ownerID, widgetID, body.Message).Scan(&sessionID); err != nil {
			c.logRequestError(r, "public webhook conversation create failed", err, "widget_key", body.WidgetKey)
			utils.JSONErr(w, http.StatusInternalServerError, "failed to create conversation")
			return
		}
	} else {
		if !publicSessionUUIDPattern.MatchString(sessionID) {
			utils.JSONErr(w, http.StatusBadRequest, "sessionId must be a UUID")
			return
		}
		if err := c.db.QueryRow(`INSERT INTO chat_conversations (id,user_id,widget_key_id,status,last_message_preview,last_message_at)
			VALUES ($1,$2,$3,'active',$4,CURRENT_TIMESTAMP)
			ON CONFLICT (id) DO UPDATE SET
				last_message_preview=EXCLUDED.last_message_preview,
				last_message_at=CURRENT_TIMESTAMP,
				updated_at=CURRENT_TIMESTAMP,
				widget_key_id=COALESCE(chat_conversations.widget_key_id,EXCLUDED.widget_key_id)
			WHERE chat_conversations.user_id=EXCLUDED.user_id
				AND chat_conversations.is_deleted=FALSE
				AND (chat_conversations.widget_key_id IS NULL OR chat_conversations.widget_key_id=EXCLUDED.widget_key_id)
			RETURNING id`,
			sessionID, ownerID, widgetID, body.Message).Scan(&sessionID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				utils.JSONErr(w, http.StatusNotFound, "session not found")
				return
			}
			c.logRequestWarn(r, "public webhook conversation upsert failed", err, "widget_key", body.WidgetKey, "session_id", sessionID)
			utils.JSONErr(w, http.StatusInternalServerError, "failed to update conversation")
			return
		}
	}
	matches, matchErr := c.pineconeQuery(ownerID, body.Message, 5)
	relevantMatches := relevantRAGMatches(matches, 0.65)
	if matchErr != nil {
		c.logRequestWarn(r, "public webhook context lookup failed", matchErr, "widget_key", body.WidgetKey, "session_id", sessionID)
	}
	answer := "I don't have information about that. Please contact support."
	if len(relevantMatches) > 0 {
		if ai, aiErr := c.openAIAnswerWithContext(body.Message, relevantMatches); aiErr == nil && strings.TrimSpace(ai) != "" {
			answer = ai
		} else if aiErr != nil {
			c.logRequestWarn(r, "public webhook response generation with context failed", aiErr, "widget_key", body.WidgetKey, "session_id", sessionID)
		}
	}
	insertResult, err := c.db.Exec(`INSERT INTO chat_messages (conversation_id,user_id,role,content,metadata)
		SELECT c.id,$2,v.role,v.content,'{}'::jsonb
		FROM chat_conversations c
		JOIN (VALUES ('user'::varchar,$3),('assistant'::varchar,$4)) AS v(role,content) ON TRUE
		WHERE c.id=$1 AND c.user_id=$2 AND c.is_deleted=FALSE AND (c.widget_key_id IS NULL OR c.widget_key_id=$5)`,
		sessionID, ownerID, body.Message, answer, widgetID)
	if err != nil {
		c.logRequestWarn(r, "public webhook message insert failed", err, "widget_key", body.WidgetKey, "session_id", sessionID)
	} else if rows, rowsErr := insertResult.RowsAffected(); rowsErr == nil && rows != 2 {
		c.requestLogger(r).Warn("public webhook message insert skipped due to session ownership mismatch",
			"widget_key", body.WidgetKey, "session_id", sessionID, "rows_affected", rows)
		utils.JSONErr(w, http.StatusNotFound, "session not found")
		return
	}
	if _, err := c.db.Exec(`UPDATE chat_conversations SET message_count=message_count+2,last_message_preview=$2,last_message_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND user_id=$3 AND is_deleted=FALSE AND (widget_key_id IS NULL OR widget_key_id=$4)`,
		sessionID, body.Message, ownerID, widgetID); err != nil {
		c.logRequestWarn(r, "public webhook conversation update failed", err, "widget_key", body.WidgetKey, "session_id", sessionID)
	}
	c.queueWidgetAnalytics(r.Context(), widgetID, "message_sent", map[string]interface{}{}, r)

	if r.URL.Query().Get("stream") == "1" {
		streamWidgetResponse(w, sessionID, answer)
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "sessionId": sessionID, "response": answer})
}

func streamWidgetResponse(w http.ResponseWriter, sessionID, answer string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		utils.JSONOK(w, map[string]interface{}{"success": true, "sessionId": sessionID, "response": answer})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	for _, chunk := range splitAnswerChunks(answer, 4) {
		writeSSEEvent(w, map[string]interface{}{
			"type":  "token",
			"token": chunk,
		})
		flusher.Flush()
	}
	writeSSEEvent(w, map[string]interface{}{
		"type":      "done",
		"sessionId": sessionID,
	})
	flusher.Flush()
}

func splitAnswerChunks(answer string, wordsPerChunk int) []string {
	text := strings.TrimSpace(answer)
	if text == "" {
		return []string{""}
	}
	if wordsPerChunk <= 0 {
		wordsPerChunk = 4
	}
	words := strings.Fields(text)
	if len(words) <= wordsPerChunk {
		return []string{text}
	}
	chunks := make([]string, 0, len(words)/wordsPerChunk+1)
	for start := 0; start < len(words); start += wordsPerChunk {
		end := start + wordsPerChunk
		if end > len(words) {
			end = len(words)
		}
		chunk := strings.Join(words[start:end], " ")
		if end < len(words) {
			chunk += " "
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func writeSSEEvent(w http.ResponseWriter, payload map[string]interface{}) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", string(b))
}

func (c *Controller) WidgetPreviewPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, "public/widget/preview.html")
}

func (c *Controller) EmbedJS(w http.ResponseWriter, r *http.Request) {
	baseURL := publicBaseURL(r)
	baseJSON, _ := json.Marshal(baseURL)
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(fmt.Sprintf(`(function(){
  var current = document.currentScript;
  var widgetKey = '';
  if (current) {
    widgetKey = current.getAttribute('data-widget-key') || current.getAttribute('widget-key') || '';
    try {
      if (!widgetKey && current.src) {
        var parsed = new URL(current.src, window.location.href);
        widgetKey = parsed.searchParams.get('widgetKey') || '';
      }
    } catch (e) {}
  }
  if (!widgetKey && typeof window !== 'undefined') {
    widgetKey = window.KONVOQ_WIDGET_KEY || '';
  }
  if (!widgetKey) {
    console.error('Konvoq embed loader: missing widgetKey. Provide data-widget-key or ?widgetKey=');
    return;
  }
  var baseURL = %s;
  var script = document.createElement('script');
  script.src = baseURL + '/api/v1/embed/' + encodeURIComponent(widgetKey) + '.js';
  script.async = true;
  document.head.appendChild(script);
})();`, baseJSON)))
}

func (c *Controller) EmbedForWidget(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(chi.URLParam(r, "widgetKey"))
	if key == "" || !strings.HasPrefix(key, "wk_") {
		utils.JSONErr(w, http.StatusNotFound, "embed script not found")
		return
	}

	var configRaw []byte
	var isActive bool
	var domainsRaw string
	if err := c.db.QueryRow(`SELECT widget_config,is_active,COALESCE(to_json(allowed_domains),'[]'::json)::text FROM widget_keys WHERE widget_key=$1`, key).Scan(&configRaw, &isActive, &domainsRaw); err != nil || !isActive {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			c.logRequestWarn(r, "embed script widget lookup failed", err, "widget_key", key)
		}
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	allowedDomains := parseAllowedDomainsJSON(domainsRaw)
	if c.cfg.IsProduction && len(allowedDomains) == 0 {
		utils.JSONErr(w, http.StatusForbidden, "widget allowed domains are not configured")
		return
	}
	if !isWidgetRequestAllowed(allowedDomains, r) {
		utils.JSONErr(w, http.StatusForbidden, "widget access denied for this domain")
		return
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		cfg = map[string]interface{}{}
	}
	if !isWidgetConfigured(cfg) {
		utils.JSONErr(w, http.StatusForbidden, "widget configuration is incomplete")
		return
	}

	baseURL := publicBaseURL(r)
	baseJSON, _ := json.Marshal(baseURL)
	keyJSON, _ := json.Marshal(key)
	cfgJSON, _ := json.Marshal(cfg)

	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(fmt.Sprintf(`(function(){
  if (window.__KONVOQ_EMBED_LOADED__ && window.__KONVOQ_EMBED_LOADED__[%s]) return;
  window.__KONVOQ_EMBED_LOADED__ = window.__KONVOQ_EMBED_LOADED__ || {};
  window.__KONVOQ_EMBED_LOADED__[%s] = true;
  var baseURL = %s;
  var widgetKey = %s;
  var config = %s || {};
  var script = document.createElement('script');
  script.src = baseURL + '/widget/konvoq-chat.js';
  script.async = true;
  script.onload = function() {
    if (document.querySelector('konvoq-chat[widget-key="' + widgetKey + '"]')) return;
    var el = document.createElement('konvoq-chat');
    el.setAttribute('api-url', baseURL + '/api/v1/webhook');
    el.setAttribute('api-base-url', baseURL);
    el.setAttribute('widget-key', widgetKey);
    var resolved = {
      'primary-color': config.primaryColor || config.sendColor || config.botColor,
      'background-color': config.backgroundColor || config.bannerColor,
      'text-color': config.textColor || config.bannerTextColor,
      'bot-name': config.botName || config.bannerText,
      'welcome-message': config.welcomeMessage || config.bannerTextParagraph || config.primaryText,
      'logo-url': config.logoUrl || config.logoIcon,
      'position': config.position,
      'border-radius': config.borderRadius,
      'font-size': config.fontSize,
      'primary-text': config.primaryText || config.welcomeMessage || config.bannerTextParagraph,
      'bot-color': config.botColor || config.primaryColor,
      'send-color': config.sendColor || config.primaryColor,
      'floating-btn-color': config.floatingBtnColor || config.primaryColor,
      'floating-btn': config.floatingBtn || config.primaryColor,
      'auto-open': config.autoOpen,
      'banner-text': config.bannerText || config.botName,
      'banner-text-color': config.bannerTextColor || config.textColor,
      'banner-color': config.bannerColor || config.backgroundColor,
      'user-chat-color': config.userChatColor,
      'close-button-color': config.closeButtonColor,
      'logo-icon': config.logoIcon || config.logoUrl,
      'banner-text-paragraph': config.bannerTextParagraph || config.welcomeMessage,
      'banner-text-paragraph-color': config.bannerTextParagraphColor,
      'plan-type': config.planType,
      'default-language': config.defaultLanguage
    };
    Object.keys(resolved).forEach(function(attr){
      var value = resolved[attr];
      if (value === undefined || value === null || value === '') return;
      el.setAttribute(attr, String(value));
    });
    document.body.appendChild(el);
  };
  document.head.appendChild(script);
})();`, keyJSON, keyJSON, baseJSON, keyJSON, cfgJSON)))
}

func (c *Controller) queueWidgetAnalytics(ctx context.Context, widgetKeyID int64, eventType string, eventData map[string]interface{}, r *http.Request) {
	if c.redis == nil || widgetKeyID <= 0 || strings.TrimSpace(eventType) == "" {
		return
	}
	if eventData == nil {
		eventData = map[string]interface{}{}
	}
	if b, marshalErr := json.Marshal(map[string]interface{}{
		"widget_key_id": widgetKeyID,
		"event_type":    eventType,
		"event_data":    eventData,
		"ip":            r.RemoteAddr,
		"user_agent":    r.UserAgent(),
		"referer":       r.Referer(),
	}); marshalErr == nil {
		_ = c.redis.RPush(ctx, "widget:analytics:buffer", string(b)).Err()
	}
}

func publicBaseURL(r *http.Request) string {
	scheme := "http"
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") || r.TLS != nil {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = "localhost:8080"
	}
	return scheme + "://" + host
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
	body.WidgetKey = strings.TrimSpace(body.WidgetKey)
	if strings.TrimSpace(body.SessionID) == "" {
		body.SessionID = utils.RandomID("sess")
	}
	var widgetID int64
	var ownerID string
	var domainsRaw string
	err := c.db.QueryRow(`SELECT id,user_id,COALESCE(to_json(allowed_domains),'[]'::json)::text FROM widget_keys WHERE widget_key=$1 AND is_active=TRUE`, body.WidgetKey).Scan(&widgetID, &ownerID, &domainsRaw)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			c.logRequestError(r, "public contact widget lookup failed", err, "widget_key", body.WidgetKey)
		}
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	allowedDomains := parseAllowedDomainsJSON(domainsRaw)
	if c.cfg.IsProduction && len(allowedDomains) == 0 {
		utils.JSONErr(w, http.StatusForbidden, "widget allowed domains are not configured")
		return
	}
	if !isWidgetRequestAllowed(allowedDomains, r) {
		utils.JSONErr(w, http.StatusForbidden, "widget access denied for this domain")
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
	body.WidgetKey = strings.TrimSpace(body.WidgetKey)
	body.SessionID = strings.TrimSpace(body.SessionID)
	if body.WidgetKey == "" || body.SessionID == "" {
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
	var domainsRaw string
	err := c.db.QueryRow(`SELECT user_id,id,COALESCE(to_json(allowed_domains),'[]'::json)::text FROM widget_keys WHERE widget_key=$1 AND is_active=TRUE`, body.WidgetKey).Scan(&ownerID, &widgetID, &domainsRaw)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			c.logRequestError(r, "public rating widget lookup failed", err, "widget_key", body.WidgetKey, "session_id", body.SessionID)
		}
		utils.JSONErr(w, http.StatusNotFound, "widget not found")
		return
	}
	allowedDomains := parseAllowedDomainsJSON(domainsRaw)
	if c.cfg.IsProduction && len(allowedDomains) == 0 {
		utils.JSONErr(w, http.StatusForbidden, "widget allowed domains are not configured")
		return
	}
	if !isWidgetRequestAllowed(allowedDomains, r) {
		utils.JSONErr(w, http.StatusForbidden, "widget access denied for this domain")
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
