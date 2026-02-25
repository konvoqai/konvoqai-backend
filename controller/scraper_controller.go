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
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	go func(userID, sourceURL string) {
		text, err := c.extractTextFromURL(sourceURL)
		if err != nil || strings.TrimSpace(text) == "" {
			return
		}
		_ = c.pineconeUpsert(userID, sourceURL, text)
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
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&sources)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM documents WHERE user_id=$1`, claims.UserID).Scan(&docs)
	matches, _ := c.pineconeQuery(claims.UserID, body.Query, 3)
	answer := "I found relevant documents but could not generate an answer right now."
	if ai, err := c.openAIAnswerWithContext(body.Query, matches); err == nil && strings.TrimSpace(ai) != "" {
		answer = ai
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
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) DeleteAllSources(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	_, _ = c.db.Exec(`DELETE FROM scraper_sources WHERE user_id=$1`, claims.UserID)
	_, _ = c.db.Exec(`DELETE FROM documents WHERE user_id=$1`, claims.UserID)
	utils.JSONOK(w, map[string]interface{}{"success": true})
}

func (c *Controller) SourceStats(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	var sources, docs int
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&sources)
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM documents WHERE user_id=$1`, claims.UserID).Scan(&docs)
	utils.JSONOK(w, map[string]interface{}{"success": true, "stats": map[string]int{"sources": sources, "documents": docs}})
}

func (c *Controller) GetSources(w http.ResponseWriter, _ *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,source_url,source_title,created_at FROM scraper_sources WHERE user_id=$1 ORDER BY created_at DESC`, claims.UserID)
	if err != nil {
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id, url string
		var title sql.NullString
		var created time.Time
		_ = rows.Scan(&id, &url, &title, &created)
		items = append(items, map[string]interface{}{"id": id, "url": url, "title": utils.NullString(title), "createdAt": created})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "sources": items})
}
