package controller

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func parseAllowedDomainsJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var domains []string
	if err := json.Unmarshal([]byte(raw), &domains); err != nil {
		return nil
	}
	return normalizeAllowedDomains(domains)
}

func extractAllowedDomainsFromPayload(payload map[string]interface{}) ([]string, bool) {
	if len(payload) == 0 {
		return nil, false
	}
	if raw, ok := payload["allowedDomains"]; ok {
		return normalizeAllowedDomainsValue(raw), true
	}
	if raw, ok := payload["allowed_domains"]; ok {
		return normalizeAllowedDomainsValue(raw), true
	}
	return nil, false
}

func normalizeAllowedDomainsValue(raw interface{}) []string {
	switch value := raw.(type) {
	case []string:
		return normalizeAllowedDomains(value)
	case []interface{}:
		items := make([]string, 0, len(value))
		for _, v := range value {
			items = append(items, asString(v))
		}
		return normalizeAllowedDomains(items)
	case string:
		if strings.Contains(value, ",") {
			return normalizeAllowedDomains(strings.Split(value, ","))
		}
		return normalizeAllowedDomains([]string{value})
	default:
		return nil
	}
}

func normalizeAllowedDomains(domains []string) []string {
	seen := make(map[string]struct{}, len(domains))
	out := make([]string, 0, len(domains))
	for _, raw := range domains {
		normalized := normalizeDomain(raw)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeDomain(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || strings.EqualFold(s, "null") {
		return ""
	}
	allowWildcard := strings.HasPrefix(s, "*.")
	if allowWildcard {
		s = strings.TrimPrefix(s, "*.")
	}
	s = strings.TrimSuffix(s, ".")
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return ""
		}
		s = u.Hostname()
	} else {
		u, err := url.Parse("https://" + s)
		if err != nil {
			return ""
		}
		s = u.Hostname()
	}
	s = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
	if s == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	if allowWildcard {
		return "*." + s
	}
	return s
}

func isWidgetRequestAllowed(allowedDomains []string, r *http.Request) bool {
	if len(allowedDomains) == 0 {
		return true
	}
	reqDomain := requestDomain(r)
	if reqDomain == "" {
		return false
	}
	for _, allowed := range allowedDomains {
		if allowed == reqDomain {
			return true
		}
		if strings.HasPrefix(allowed, "*.") {
			base := strings.TrimPrefix(allowed, "*.")
			if reqDomain == base || strings.HasSuffix(reqDomain, "."+base) {
				return true
			}
		}
	}
	return false
}

func requestDomain(r *http.Request) string {
	candidates := []string{
		strings.TrimSpace(r.Header.Get("Origin")),
		strings.TrimSpace(r.Referer()),
	}
	for _, candidate := range candidates {
		if candidate == "" || strings.EqualFold(candidate, "null") {
			continue
		}
		parsed, err := url.Parse(candidate)
		if err != nil {
			continue
		}
		host := normalizeDomain(parsed.Hostname())
		if host != "" {
			return host
		}
	}
	return ""
}
