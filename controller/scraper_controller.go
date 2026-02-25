package controller

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"konvoq-backend/utils"
)

func (c *Controller) Scrape(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body struct {
		URL string `json:"url"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.URL) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "url is required")
		return
	}
	_, err := c.db.Exec(`INSERT INTO scraper_sources (user_id,source_url,source_title) VALUES ($1,$2,$2) ON CONFLICT (user_id,source_url) DO UPDATE SET source_title=EXCLUDED.source_title`,
		claims.UserID, strings.TrimSpace(body.URL))
	if err != nil {
		c.logRequestError(r, "scrape source upsert failed", err, "user_id", claims.UserID, "url", strings.TrimSpace(body.URL))
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	go func(userID, sourceURL string) {
		text, err := c.extractTextFromURL(sourceURL)
		if err != nil || strings.TrimSpace(text) == "" {
			if err != nil {
				c.logger.Warn("scrape extraction failed", "user_id", userID, "url", sourceURL, "error", err)
			}
			return
		}
		if err := c.pineconeUpsert(userID, sourceURL, text); err != nil {
			c.logger.Warn("scrape index upsert failed", "user_id", userID, "url", sourceURL, "error", err)
		}
	}(claims.UserID, strings.TrimSpace(body.URL))
	utils.JSONOK(w, map[string]interface{}{"success": true, "message": "scrape queued", "url": body.URL})
}

func (c *Controller) QueryDocuments(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var body struct {
		Query string `json:"query"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.Query) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "query is required")
		return
	}
	var sources, docs int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&sources); err != nil {
		c.logRequestWarn(r, "query documents source count failed", err, "user_id", claims.UserID)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM documents WHERE user_id=$1`, claims.UserID).Scan(&docs); err != nil {
		c.logRequestWarn(r, "query documents count failed", err, "user_id", claims.UserID)
	}
	matches, matchErr := c.pineconeQuery(claims.UserID, body.Query, 3)
	if matchErr != nil {
		c.logRequestWarn(r, "document context lookup failed", matchErr, "user_id", claims.UserID)
	}
	answer := "I found relevant documents but could not generate an answer right now."
	if ai, err := c.openAIAnswerWithContext(body.Query, matches); err == nil && strings.TrimSpace(ai) != "" {
		answer = ai
	} else if err != nil {
		c.logRequestWarn(r, "document response generation failed", err, "user_id", claims.UserID)
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "answer": answer, "documentsSearched": sources + docs, "matches": matches})
}

func (c *Controller) DeleteSource(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		var body struct {
			URL string `json:"url"`
		}
		_ = utils.DecodeJSON(r, &body)
		rawURL = body.URL
	}
	if strings.TrimSpace(rawURL) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "url is required")
		return
	}
	_, err := c.db.Exec(`DELETE FROM scraper_sources WHERE user_id=$1 AND source_url=$2`, claims.UserID, strings.TrimSpace(rawURL))
	if err != nil {
		c.logRequestError(r, "delete source failed", err, "user_id", claims.UserID, "url", strings.TrimSpace(rawURL))
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) DeleteAllSources(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	if _, err := c.db.Exec(`DELETE FROM scraper_sources WHERE user_id=$1`, claims.UserID); err != nil {
		c.logRequestError(r, "delete all sources failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if _, err := c.db.Exec(`DELETE FROM documents WHERE user_id=$1`, claims.UserID); err != nil {
		c.logRequestError(r, "delete all documents failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) SourceStats(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	var sources, docs int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&sources); err != nil {
		c.logRequestWarn(r, "source stats source count failed", err, "user_id", claims.UserID)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM documents WHERE user_id=$1`, claims.UserID).Scan(&docs); err != nil {
		c.logRequestWarn(r, "source stats document count failed", err, "user_id", claims.UserID)
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "stats": map[string]int{"sources": sources, "documents": docs}})
}

func (c *Controller) GetSources(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,source_url,source_title,created_at FROM scraper_sources WHERE user_id=$1 ORDER BY created_at DESC`, claims.UserID)
	if err != nil {
		c.logRequestError(r, "get sources query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id, url string
		var title sql.NullString
		var created time.Time
		if err := rows.Scan(&id, &url, &title, &created); err != nil {
			c.logRequestWarn(r, "get sources row scan failed", err, "user_id", claims.UserID)
			continue
		}
		items = append(items, map[string]interface{}{"id": id, "url": url, "title": utils.NullString(title), "createdAt": created})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "sources": items})
}
