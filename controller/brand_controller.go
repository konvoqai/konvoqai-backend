package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"konvoq-backend/utils"
)

// ── Brand extraction regexes ────────────────────────────────────────────────

var (
	brandThemeColorPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)<meta[^>]+name=["']theme-color["'][^>]+content=["']([^"']+)["']`),
		regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+name=["']theme-color["']`),
		regexp.MustCompile(`(?i)<meta[^>]+name=["']msapplication-TileColor["'][^>]+content=["']([^"']+)["']`),
		regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+name=["']msapplication-TileColor["']`),
	}
	brandSiteNamePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)<meta[^>]+property=["']og:site_name["'][^>]+content=["']([^"']+)["']`),
		regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:site_name["']`),
	}
	brandOgTitlePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']+)["']`),
		regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:title["']`),
	}
	brandPageTitlePattern  = regexp.MustCompile(`(?i)<title[^>]*>([^<]{1,120})</title>`)
	brandCSSVarPattern     = regexp.MustCompile(`(?i)--(?:primary|brand|accent|color-primary|main-color)[^:]*:\s*(#[0-9a-fA-F]{3,8})\b`)
	brandHexColorPattern   = regexp.MustCompile(`(?i)^#([0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$`)
	brandTitleSuffixPattern = regexp.MustCompile(`\s*[|\-–—]\s*.+$`)
	brandCodeFencePattern   = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")
)

func brandExtractMeta(html string, patterns []*regexp.Regexp) string {
	for _, pat := range patterns {
		if m := pat.FindStringSubmatch(html); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

func brandIsValidHex(v string) bool {
	return brandHexColorPattern.MatchString(strings.TrimSpace(v))
}

type brandHints struct {
	SiteName      string
	ExtractedColor string
}

func parseBrandHints(html string) brandHints {
	var h brandHints

	// Color: theme-color meta → msapplication-TileColor → CSS var
	color := brandExtractMeta(html, brandThemeColorPatterns)
	if brandIsValidHex(color) {
		h.ExtractedColor = color
	} else if m := brandCSSVarPattern.FindStringSubmatch(html); len(m) > 1 && brandIsValidHex(m[1]) {
		h.ExtractedColor = m[1]
	}

	// Site name: og:site_name → <title> → og:title
	siteName := brandExtractMeta(html, brandSiteNamePatterns)
	if siteName == "" {
		if m := brandPageTitlePattern.FindStringSubmatch(html); len(m) > 1 {
			siteName = strings.TrimSpace(brandTitleSuffixPattern.ReplaceAllString(strings.TrimSpace(m[1]), ""))
		}
	}
	if siteName == "" {
		raw := brandExtractMeta(html, brandOgTitlePatterns)
		siteName = strings.TrimSpace(brandTitleSuffixPattern.ReplaceAllString(raw, ""))
	}
	h.SiteName = siteName
	return h
}

// ── Handler ────────────────────────────────────────────────────────────────

func (c *Controller) BrandExtract(w http.ResponseWriter, r *http.Request, _ TokenClaims, _ UserRecord) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		utils.JSONErr(w, http.StatusBadRequest, "url param required")
		return
	}

	targetURL, err := c.validateScrapeTarget(rawURL)
	if err != nil {
		utils.JSONOK(w, map[string]interface{}{"success": true, "brand": nil})
		return
	}

	// Fetch up to 80 KB of HTML
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		utils.JSONOK(w, map[string]interface{}{"success": true, "brand": nil})
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; KonvoqBot/1.0; +https://konvoq.ai)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		utils.JSONOK(w, map[string]interface{}{"success": true, "brand": nil})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 || !strings.Contains(resp.Header.Get("Content-Type"), "html") {
		utils.JSONOK(w, map[string]interface{}{"success": true, "brand": nil})
		return
	}

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 80*1024))
	hints := parseBrandHints(string(raw))

	brand := c.aiBrandColors(hints)
	utils.JSONOK(w, map[string]interface{}{"success": true, "brand": brand})
}

// ── AI color suggestion ────────────────────────────────────────────────────

func (c *Controller) aiBrandColors(hints brandHints) map[string]interface{} {
	if strings.TrimSpace(c.cfg.OpenAIAPIKey) == "" {
		return buildFallbackBrand(hints)
	}

	colorHint := "none extracted"
	if hints.ExtractedColor != "" {
		colorHint = hints.ExtractedColor
	}
	nameHint := "unknown website"
	if hints.SiteName != "" {
		nameHint = hints.SiteName
	}

	prompt := fmt.Sprintf(`You are a UI/UX color expert specializing in chat widgets.

Website name: %s
Primary color extracted from HTML: %s

Suggest a polished, professional color palette for a chat widget. Return ONLY a JSON object with no explanation or markdown fences:

{
  "primaryColor": "#hex (vibrant main color for header and buttons — use extracted if valid, else pick professionally)",
  "backgroundColor": "#hex (chat window background — use #0f1013 for dark or #ffffff for light)",
  "textColor": "#hex (chat text color — #ffffff for dark bg, #111111 for light bg)",
  "botName": "friendly assistant name ending in AI (max 30 chars)",
  "welcomeMessage": "warm greeting message (max 80 chars)"
}

Rules:
- If the extracted color is a valid hex color, prefer it as primaryColor
- Ensure backgroundColor and textColor have high contrast (WCAG AA)
- botName should include the website name naturally (e.g. "Acme AI" or "Acme Support AI")
- Return valid JSON only, no extra text`, nameHint, colorHint)

	result, err := c.openAIBrandChat(prompt)
	if err != nil || result == "" {
		c.logger.Warn("brand extract ai failed, using fallback", "error", err)
		return buildFallbackBrand(hints)
	}

	// Strip markdown fences if present
	if m := brandCodeFencePattern.FindStringSubmatch(result); len(m) > 1 {
		result = m[1]
	}

	var parsed map[string]string
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		c.logger.Warn("brand extract ai json parse failed", "error", err, "raw", result)
		return buildFallbackBrand(hints)
	}

	brand := map[string]interface{}{}
	if v := parsed["primaryColor"]; brandIsValidHex(v) {
		brand["primaryColor"] = v
	}
	if v := parsed["backgroundColor"]; brandIsValidHex(v) {
		brand["backgroundColor"] = v
	}
	if v := parsed["textColor"]; brandIsValidHex(v) {
		brand["textColor"] = v
	}
	if v := parsed["botName"]; v != "" {
		brand["botName"] = v
	}
	if v := parsed["welcomeMessage"]; v != "" {
		brand["welcomeMessage"] = v
	}
	if len(brand) == 0 {
		return buildFallbackBrand(hints)
	}
	return brand
}

func buildFallbackBrand(hints brandHints) map[string]interface{} {
	brand := map[string]interface{}{
		"primaryColor":    "#5b8cff",
		"backgroundColor": "#0f1013",
		"textColor":       "#ffffff",
	}
	if hints.ExtractedColor != "" {
		brand["primaryColor"] = hints.ExtractedColor
	}
	if hints.SiteName != "" {
		name := hints.SiteName
		if len(name) > 24 {
			name = name[:24]
		}
		botName := name + " AI"
		brand["botName"] = botName
		brand["welcomeMessage"] = "Hi! I'm " + botName + ". How can I help you today?"
	}
	return brand
}

func (c *Controller) openAIBrandChat(prompt string) (string, error) {
	payload := map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("openai status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
