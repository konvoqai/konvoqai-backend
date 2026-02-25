package controller

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"konvoq-backend/utils"
)

func (c *Controller) Chat(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body struct {
		Message   string `json:"message"`
		SessionID string `json:"sessionId"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Message) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "message is required")
		return
	}
	var used int
	var limit sql.NullInt64
	err := c.db.QueryRow(`UPDATE users SET conversations_used=conversations_used+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND (conversations_limit IS NULL OR conversations_used < conversations_limit) RETURNING conversations_used,conversations_limit`,
		claims.UserID).Scan(&used, &limit)
	if err == sql.ErrNoRows {
		utils.JSONErr(w, http.StatusPaymentRequired, "conversation limit reached")
		return
	}
	if err != nil {
		c.logRequestError(r, "chat usage update failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	convID := strings.TrimSpace(body.SessionID)
	if convID == "" {
		err = c.db.QueryRow(`INSERT INTO chat_conversations (user_id,status,last_message_preview,last_message_at) VALUES ($1,'active',$2,CURRENT_TIMESTAMP) RETURNING id`,
			claims.UserID, body.Message).Scan(&convID)
	} else {
		_, err = c.db.Exec(`INSERT INTO chat_conversations (id,user_id,status,last_message_preview,last_message_at) VALUES ($1,$2,'active',$3,CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET last_message_preview=EXCLUDED.last_message_preview,last_message_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`,
			convID, claims.UserID, body.Message)
	}
	if err != nil {
		c.logRequestError(r, "chat conversation create/update failed", err, "user_id", claims.UserID, "session_id", convID)
		utils.JSONErr(w, http.StatusInternalServerError, "failed to create conversation")
		return
	}
	answer := "I couldn't generate a response right now. Please try again."
	if ragMatches, ragErr := c.pineconeQuery(claims.UserID, body.Message, 3); ragErr == nil && len(ragMatches) > 0 {
		if ai, aiErr := c.openAIAnswerWithContext(body.Message, ragMatches); aiErr == nil && strings.TrimSpace(ai) != "" {
			answer = ai
		} else if aiErr != nil {
			c.logRequestWarn(r, "chat response generation with context failed", aiErr, "user_id", claims.UserID, "session_id", convID)
		}
	} else {
		if ragErr != nil {
			c.logRequestWarn(r, "chat context lookup failed", ragErr, "user_id", claims.UserID, "session_id", convID)
		}
		if ai, aiErr := c.openAIChat(body.Message); aiErr == nil && strings.TrimSpace(ai) != "" {
			answer = ai
		} else if aiErr != nil {
			c.logRequestWarn(r, "chat response generation failed", aiErr, "user_id", claims.UserID, "session_id", convID)
		}
	}
	if _, err := c.db.Exec(`INSERT INTO chat_messages (conversation_id,user_id,role,content,metadata) VALUES ($1,$2,'user',$3,'{}'::jsonb),($1,$2,'assistant',$4,'{}'::jsonb)`,
		convID, claims.UserID, body.Message, answer); err != nil {
		c.logRequestWarn(r, "chat message insert failed", err, "user_id", claims.UserID, "session_id", convID)
	}
	if _, err := c.db.Exec(`UPDATE chat_conversations SET message_count=message_count+2,last_message_preview=$2,last_message_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`,
		convID, body.Message); err != nil {
		c.logRequestWarn(r, "chat conversation metadata update failed", err, "user_id", claims.UserID, "session_id", convID)
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "sessionId": convID, "response": answer, "usage": map[string]interface{}{"conversationsUsed": used, "conversationsLimit": utils.NullableInt64(limit)}})
}

func (c *Controller) ChatSessions(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,status,message_count,last_message_preview,last_message_at,created_at,updated_at FROM chat_conversations WHERE user_id=$1 AND is_deleted=FALSE ORDER BY last_message_at DESC`,
		claims.UserID)
	if err != nil {
		c.logRequestError(r, "chat sessions query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id, status string
		var cnt int
		var preview sql.NullString
		var lm, created, updated time.Time
		if err := rows.Scan(&id, &status, &cnt, &preview, &lm, &created, &updated); err != nil {
			c.logRequestWarn(r, "chat sessions row scan failed", err, "user_id", claims.UserID)
			continue
		}
		items = append(items, map[string]interface{}{
			"id": id, "status": status, "messageCount": cnt,
			"lastMessagePreview": utils.NullString(preview), "lastMessageAt": lm,
			"createdAt": created, "updatedAt": updated,
		})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "sessions": items})
}

func (c *Controller) ChatSession(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	sid := chi.URLParam(r, "id")
	var exists bool
	if err := c.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM chat_conversations WHERE id=$1 AND user_id=$2 AND is_deleted=FALSE)`, sid, claims.UserID).Scan(&exists); err != nil {
		c.logRequestError(r, "chat session existence check failed", err, "user_id", claims.UserID, "session_id", sid)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !exists {
		utils.JSONErr(w, http.StatusNotFound, "chat session not found")
		return
	}
	rows, err := c.db.Query(`SELECT role,content,created_at FROM chat_messages WHERE conversation_id=$1 ORDER BY created_at ASC,id ASC`, sid)
	if err != nil {
		c.logRequestError(r, "chat session messages query failed", err, "user_id", claims.UserID, "session_id", sid)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	msgs := []map[string]interface{}{}
	for rows.Next() {
		var role, content string
		var created time.Time
		if err := rows.Scan(&role, &content, &created); err != nil {
			c.logRequestWarn(r, "chat session messages row scan failed", err, "user_id", claims.UserID, "session_id", sid)
			continue
		}
		msgs = append(msgs, map[string]interface{}{"role": role, "content": content, "createdAt": created})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "session": map[string]interface{}{"id": sid, "messages": msgs}})
}

func (c *Controller) ClearChatSession(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	sid := chi.URLParam(r, "id")
	if _, err := c.db.Exec(`UPDATE chat_conversations SET is_deleted=TRUE,status='closed',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND user_id=$2`, sid, claims.UserID); err != nil {
		c.logRequestError(r, "clear chat session failed", err, "user_id", claims.UserID, "session_id", sid)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) ClearUserSessions(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	if _, err := c.db.Exec(`UPDATE chat_conversations SET is_deleted=TRUE,status='closed',updated_at=CURRENT_TIMESTAMP WHERE user_id=$1`, claims.UserID); err != nil {
		c.logRequestError(r, "clear user chat sessions failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}
