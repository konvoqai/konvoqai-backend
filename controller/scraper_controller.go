package controller

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"konvoq-backend/utils"
)

var (
	htmlTitleRegex   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlStripRegex   = regexp.MustCompile(`(?s)<[^>]+>`)
	scriptStyleRegex = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>`)
	hrefAttrRegex    = regexp.MustCompile(`(?is)href\s*=\s*(?:"([^"]+)"|'([^']+)'|([^\s"'<>]+))`)
)

type scrapedPage struct {
	URL   string
	Title string
	Text  string
}

func (c *Controller) Scrape(w http.ResponseWriter, r *http.Request, claims TokenClaims, user UserRecord) {
	var body struct {
		URL string `json:"url"`
	}
	if err := utils.DecodeJSON(r, &body); err != nil || strings.TrimSpace(body.URL) == "" {
		utils.JSONErr(w, http.StatusBadRequest, "url is required")
		return
	}
	sourceURL, err := normalizeScrapeURL(strings.TrimSpace(body.URL))
	if err != nil {
		utils.JSONErr(w, http.StatusBadRequest, "invalid url")
		return
	}
	limits := limitsForPlan(user.PlanType)

	var currentSourcePages int
	if err := c.db.QueryRow(`SELECT COALESCE(scraped_pages, 0) FROM scraper_sources WHERE user_id=$1 AND source_url=$2`, claims.UserID, sourceURL).Scan(&currentSourcePages); err != nil {
		if err != sql.ErrNoRows {
			c.logRequestError(r, "scrape source page usage query failed", err, "user_id", claims.UserID, "url", sourceURL)
			utils.JSONErr(w, http.StatusInternalServerError, "db error")
			return
		}
		currentSourcePages = 0
	}

	var currentPages int
	if err := c.db.QueryRow(`SELECT COALESCE(SUM(scraped_pages), 0) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&currentPages); err != nil {
		c.logRequestError(r, "scrape usage query failed", err, "user_id", claims.UserID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	pagesUsedWithoutCurrent := currentPages - currentSourcePages
	if pagesUsedWithoutCurrent < 0 {
		pagesUsedWithoutCurrent = 0
	}
	remainingPages := limits.ScrapedPages - pagesUsedWithoutCurrent
	if remainingPages <= 0 {
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      false,
			"message":      "scraped page limit reached for your plan",
			"limitReached": true,
			"usage": map[string]interface{}{
				"used":  currentPages,
				"limit": limits.ScrapedPages,
			},
		})
		return
	}

	_, err = c.db.Exec(`INSERT INTO scraper_sources (user_id,source_url,source_title,scraped_pages) VALUES ($1,$2,$2,$3)
		ON CONFLICT (user_id,source_url) DO UPDATE SET source_title=EXCLUDED.source_title,updated_at=CURRENT_TIMESTAMP`,
		claims.UserID, sourceURL, currentSourcePages)
	if err != nil {
		c.logRequestError(r, "scrape source upsert failed", err, "user_id", claims.UserID, "url", sourceURL)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	var jobID string
	if err := c.db.QueryRow(`INSERT INTO scrape_jobs (user_id,source_url,status,progress,message,started_at)
		VALUES ($1,$2,'queued',5,'Queued for scraping',CURRENT_TIMESTAMP) RETURNING id`,
		claims.UserID, sourceURL).Scan(&jobID); err != nil {
		c.logRequestError(r, "scrape job insert failed", err, "user_id", claims.UserID, "url", sourceURL)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	go c.runScrapeJob(claims.UserID, sourceURL, jobID, remainingPages)

	utils.JSONOK(w, map[string]interface{}{
		"success":  true,
		"message":  "scrape started",
		"url":      sourceURL,
		"jobId":    jobID,
		"status":   "queued",
		"progress": 5,
		"pageLimit": map[string]int{
			"maxForThisRun": remainingPages,
			"plan":          limits.ScrapedPages,
		},
	})
}

func (c *Controller) runScrapeJob(userID, sourceURL, jobID string, maxPages int) {
	if maxPages <= 0 {
		c.updateScrapeJob(jobID, "failed", 100, "Scraping failed", "scraped page limit reached")
		return
	}

	c.updateScrapeJob(jobID, "scraping", 20, fmt.Sprintf("Crawling up to %d pages", maxPages), "")
	pages, err := c.crawlWebsite(sourceURL, maxPages)
	if err != nil {
		c.logger.Warn("scrape extraction failed", "user_id", userID, "url", sourceURL, "job_id", jobID, "error", err)
		c.updateScrapeJob(jobID, "failed", 100, "Scraping failed", err.Error())
		return
	}
	if len(pages) == 0 {
		errMsg := "no text content found at url"
		c.logger.Warn("scrape extraction produced empty content", "user_id", userID, "url", sourceURL, "job_id", jobID)
		c.updateScrapeJob(jobID, "failed", 100, "Scraping failed", errMsg)
		return
	}

	c.updateScrapeJob(jobID, "scraping", 55, "Chunking scraped pages", "")
	chunks := c.buildRAGChunks(userID, pages)
	if len(chunks) == 0 {
		c.updateScrapeJob(jobID, "failed", 100, "Scraping failed", "no chunks generated from scraped pages")
		return
	}

	c.updateScrapeJob(jobID, "indexing", 70, "Resetting vector namespace", "")
	if err := c.pineconeDeleteNamespace(userID); err != nil {
		c.logger.Warn("scrape namespace reset failed", "user_id", userID, "url", sourceURL, "job_id", jobID, "error", err)
		c.updateScrapeJob(jobID, "failed", 100, "Indexing failed", err.Error())
		return
	}

	c.updateScrapeJob(jobID, "indexing", 85, "Indexing content", "")
	if err := c.pineconeUpsertChunks(userID, chunks); err != nil {
		c.logger.Warn("scrape index upsert failed", "user_id", userID, "url", sourceURL, "job_id", jobID, "error", err)
		c.updateScrapeJob(jobID, "failed", 100, "Indexing failed", err.Error())
		return
	}

	title := sourceURL
	if strings.TrimSpace(pages[0].Title) != "" {
		title = strings.TrimSpace(pages[0].Title)
	}
	if _, err := c.db.Exec(`UPDATE scraper_sources SET source_title=$3,scraped_pages=$4,updated_at=CURRENT_TIMESTAMP WHERE user_id=$1 AND source_url=$2`,
		userID, sourceURL, title, len(pages)); err != nil {
		c.logger.Warn("scrape source page update failed", "user_id", userID, "url", sourceURL, "job_id", jobID, "error", err)
	}

	c.updateScrapeJob(jobID, "done", 100, "Scraping complete", "")
}

func (c *Controller) crawlWebsite(startURL string, maxPages int) ([]scrapedPage, error) {
	start, err := normalizeScrapeURL(startURL)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(start)
	if err != nil {
		return nil, fmt.Errorf("invalid url")
	}
	baseHost := strings.ToLower(base.Hostname())

	queue := []string{start}
	visited := make(map[string]struct{})
	pages := make([]scrapedPage, 0, maxPages)

	for len(queue) > 0 && len(pages) < maxPages {
		current := queue[0]
		queue = queue[1:]
		if _, seen := visited[current]; seen {
			continue
		}
		visited[current] = struct{}{}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
		if reqErr != nil {
			cancel()
			continue
		}
		req.Header.Set("User-Agent", "KonvoqCrawler/1.0")
		resp, reqErr := http.DefaultClient.Do(req)
		if reqErr != nil {
			cancel()
			c.logger.Warn("scrape crawl request failed", "url", current, "error", reqErr)
			continue
		}

		contentType := strings.ToLower(resp.Header.Get("Content-Type"))
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		cancel()
		if readErr != nil {
			c.logger.Warn("scrape crawl read failed", "url", current, "error", readErr)
			continue
		}
		if resp.StatusCode >= 300 {
			c.logger.Warn("scrape crawl non-success status", "url", current, "status_code", resp.StatusCode)
			continue
		}

		raw := string(body)
		text := normalizeWhitespace(raw)
		title := ""
		links := []string{}
		if strings.Contains(contentType, "text/html") || strings.Contains(strings.ToLower(raw), "<html") {
			text = stripHTML(raw)
			title = extractHTMLTitle(raw)
			links = extractInternalLinks(baseHost, current, raw)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}

		pages = append(pages, scrapedPage{
			URL:   current,
			Title: title,
			Text:  text,
		})

		for _, link := range links {
			if _, seen := visited[link]; !seen {
				queue = append(queue, link)
			}
		}
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("no crawlable pages found")
	}

	return pages, nil
}

func normalizeScrapeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid url")
	}
	u.Fragment = ""
	u.RawQuery = ""
	u.User = nil
	u.Host = strings.ToLower(strings.TrimSpace(u.Host))
	if u.Path == "" {
		u.Path = "/"
	}
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
		if u.Path == "" {
			u.Path = "/"
		}
	}
	return u.String(), nil
}

func extractHTMLTitle(raw string) string {
	match := htmlTitleRegex.FindStringSubmatch(raw)
	if len(match) < 2 {
		return ""
	}
	return normalizeWhitespace(html.UnescapeString(match[1]))
}

func stripHTML(raw string) string {
	withoutScripts := scriptStyleRegex.ReplaceAllString(raw, " ")
	withoutTags := htmlStripRegex.ReplaceAllString(withoutScripts, " ")
	return normalizeWhitespace(html.UnescapeString(withoutTags))
}

func normalizeWhitespace(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func extractInternalLinks(baseHost, currentURL, rawHTML string) []string {
	current, err := url.Parse(currentURL)
	if err != nil {
		return nil
	}
	unique := make(map[string]struct{})
	matches := hrefAttrRegex.FindAllStringSubmatch(rawHTML, -1)
	for _, match := range matches {
		target := ""
		for i := 1; i < len(match); i++ {
			if strings.TrimSpace(match[i]) != "" {
				target = strings.TrimSpace(match[i])
				break
			}
		}
		if target == "" {
			continue
		}
		target = html.UnescapeString(target)
		lower := strings.ToLower(target)
		if strings.HasPrefix(lower, "#") ||
			strings.HasPrefix(lower, "mailto:") ||
			strings.HasPrefix(lower, "tel:") ||
			strings.HasPrefix(lower, "javascript:") {
			continue
		}

		ref, err := url.Parse(target)
		if err != nil {
			continue
		}
		absolute := current.ResolveReference(ref)
		if !strings.EqualFold(absolute.Hostname(), baseHost) {
			continue
		}
		if shouldSkipCrawlPath(absolute.Path) {
			continue
		}
		normalized, err := normalizeScrapeURL(absolute.String())
		if err != nil {
			continue
		}
		unique[normalized] = struct{}{}
	}

	links := make([]string, 0, len(unique))
	for link := range unique {
		links = append(links, link)
	}
	sort.Strings(links)
	return links
}

func shouldSkipCrawlPath(rawPath string) bool {
	ext := strings.ToLower(path.Ext(strings.TrimSpace(rawPath)))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".bmp", ".tiff",
		".css", ".js", ".map", ".woff", ".woff2", ".ttf", ".otf",
		".pdf", ".zip", ".tar", ".gz", ".rar", ".7z",
		".mp3", ".wav", ".ogg", ".mp4", ".mov", ".avi", ".webm":
		return true
	default:
		return false
	}
}

func (c *Controller) buildRAGChunks(userID string, pages []scrapedPage) []ragChunk {
	widgetKey := c.userWidgetKey(userID)
	chunks := make([]ragChunk, 0, len(pages)*2)
	for _, page := range pages {
		pageChunks := chunkText(page.Text, 500, 75)
		for i, chunk := range pageChunks {
			chunks = append(chunks, ragChunk{
				URL:        page.URL,
				PageTitle:  page.Title,
				Text:       chunk,
				ChunkIndex: i,
				WidgetKey:  widgetKey,
			})
		}
	}
	return chunks
}

func chunkText(raw string, chunkSize int, overlap int) []string {
	words := strings.Fields(strings.TrimSpace(raw))
	if len(words) == 0 {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 500
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = 75
	}
	step := chunkSize - overlap
	if step <= 0 {
		step = chunkSize
	}

	out := make([]string, 0, len(words)/step+1)
	for start := 0; start < len(words); start += step {
		end := start + chunkSize
		if end > len(words) {
			end = len(words)
		}
		out = append(out, strings.Join(words[start:end], " "))
		if end == len(words) {
			break
		}
	}
	return out
}

func (c *Controller) updateScrapeJob(jobID, status string, progress int, message, errMsg string) {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	completedAt := "NULL"
	if status == "done" || status == "failed" {
		completedAt = "CURRENT_TIMESTAMP"
	}
	if _, err := c.db.Exec(`UPDATE scrape_jobs
		SET status=$2,progress=$3,message=$4,error_message=$5,
		    completed_at=`+completedAt+`,updated_at=CURRENT_TIMESTAMP
		WHERE id=$1`, jobID, status, progress, utils.Nullable(message), utils.Nullable(errMsg)); err != nil {
		c.logger.Warn("scrape job update failed", "job_id", jobID, "status", status, "error", err)
	}
}

func (c *Controller) GetScrapeJob(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	jobID := strings.TrimSpace(chi.URLParam(r, "id"))
	if jobID == "" {
		utils.JSONErr(w, http.StatusBadRequest, "job id is required")
		return
	}

	var sourceURL, status string
	var progress int
	var message, errMsg sql.NullString
	var createdAt, updatedAt time.Time
	var startedAt, completedAt sql.NullTime

	err := c.db.QueryRow(`SELECT source_url,status,progress,message,error_message,created_at,started_at,completed_at,updated_at
		FROM scrape_jobs WHERE id=$1 AND user_id=$2`,
		jobID, claims.UserID).Scan(
		&sourceURL, &status, &progress, &message, &errMsg,
		&createdAt, &startedAt, &completedAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			utils.JSONErr(w, http.StatusNotFound, "scrape job not found")
			return
		}
		c.logRequestError(r, "get scrape job query failed", err, "user_id", claims.UserID, "job_id", jobID)
		utils.JSONErr(w, http.StatusInternalServerError, "db error")
		return
	}

	utils.JSONOK(w, map[string]interface{}{
		"success": true,
		"job": map[string]interface{}{
			"id":          jobID,
			"url":         sourceURL,
			"status":      status,
			"progress":    progress,
			"message":     utils.NullString(message),
			"error":       utils.NullString(errMsg),
			"createdAt":   createdAt,
			"startedAt":   utils.NullTime(startedAt),
			"completedAt": utils.NullTime(completedAt),
			"updatedAt":   updatedAt,
		},
	})
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
	matches, matchErr := c.pineconeQuery(claims.UserID, body.Query, 5)
	relevantMatches := relevantRAGMatches(matches, 0.65)
	if matchErr != nil {
		c.logRequestWarn(r, "document context lookup failed", matchErr, "user_id", claims.UserID)
	}
	answer := "I don't have information about that. Please contact support."
	if len(relevantMatches) > 0 {
		if ai, err := c.openAIAnswerWithContext(body.Query, relevantMatches); err == nil && strings.TrimSpace(ai) != "" {
			answer = ai
		} else if err != nil {
			c.logRequestWarn(r, "document response generation failed", err, "user_id", claims.UserID)
		}
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "answer": answer, "documentsSearched": sources + docs, "matches": relevantMatches})
}

func relevantRAGMatches(matches []map[string]interface{}, minScore float64) []map[string]interface{} {
	if minScore <= 0 {
		minScore = 0.65
	}
	filtered := make([]map[string]interface{}, 0, len(matches))
	for _, match := range matches {
		score, ok := ragMatchScore(match)
		if !ok || score < minScore {
			continue
		}
		filtered = append(filtered, match)
	}
	return filtered
}

func ragMatchScore(match map[string]interface{}) (float64, bool) {
	raw, exists := match["score"]
	if !exists {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		if parsed, err := v.Float64(); err == nil {
			return parsed, true
		}
	}
	return 0, false
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
	var sources, docs, pendingJobs, scrapedPages int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&sources); err != nil {
		c.logRequestWarn(r, "source stats source count failed", err, "user_id", claims.UserID)
	}
	if err := c.db.QueryRow(`SELECT COALESCE(SUM(scraped_pages), 0) FROM scraper_sources WHERE user_id=$1`, claims.UserID).Scan(&scrapedPages); err != nil {
		c.logRequestWarn(r, "source stats scraped pages sum failed", err, "user_id", claims.UserID)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM documents WHERE user_id=$1`, claims.UserID).Scan(&docs); err != nil {
		c.logRequestWarn(r, "source stats document count failed", err, "user_id", claims.UserID)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM scrape_jobs WHERE user_id=$1 AND status IN ('queued','scraping','indexing')`, claims.UserID).Scan(&pendingJobs); err != nil {
		c.logRequestWarn(r, "source stats scrape jobs count failed", err, "user_id", claims.UserID)
	}
	utils.JSONOK(w, map[string]interface{}{
		"success": true,
		"stats": map[string]int{
			"sources":      sources,
			"scrapedPages": scrapedPages,
			"documents":    docs,
			"pendingJobs":  pendingJobs,
		},
	})
}

func (c *Controller) GetSources(w http.ResponseWriter, r *http.Request, claims TokenClaims, _ UserRecord) {
	rows, err := c.db.Query(`SELECT id,source_url,source_title,scraped_pages,created_at FROM scraper_sources WHERE user_id=$1 ORDER BY created_at DESC`, claims.UserID)
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
		var scrapedPages int
		var created time.Time
		if err := rows.Scan(&id, &url, &title, &scrapedPages, &created); err != nil {
			c.logRequestWarn(r, "get sources row scan failed", err, "user_id", claims.UserID)
			continue
		}
		items = append(items, map[string]interface{}{"id": id, "url": url, "title": utils.NullString(title), "scrapedPages": scrapedPages, "createdAt": created})
	}
	utils.JSONOK(w, map[string]interface{}{"success": true, "sources": items})
}
