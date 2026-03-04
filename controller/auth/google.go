package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

type Handler struct {
	clientID    string
	redirectURL string
}

func New(clientID, redirectURL string) *Handler {
	return &Handler{
		clientID:    strings.TrimSpace(clientID),
		redirectURL: strings.TrimSpace(redirectURL),
	}
}

func (h *Handler) Google(w http.ResponseWriter, r *http.Request) {
	if h.clientID == "" || h.redirectURL == "" {
		jsonErr(w, http.StatusServiceUnavailable, "google oauth is not configured")
		return
	}
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		state = randomGoogleState()
	}
	q := url.Values{}
	q.Set("client_id", h.clientID)
	q.Set("redirect_uri", h.redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("access_type", "online")
	q.Set("include_granted_scopes", "true")
	q.Set("prompt", "consent")
	q.Set("state", state)

	jsonOK(w, map[string]interface{}{
		"success": true,
		"authUrl": "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode(),
		"state":   state,
	})
}

func randomGoogleState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "oauth-state"
	}
	return hex.EncodeToString(b)
}

func (h *Handler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]interface{}{
		"success": true,
		"state":   r.URL.Query().Get("state"),
		"code":    r.URL.Query().Get("code"),
		"message": "exchange the auth code on your frontend and call /api/auth/google/verify with idToken",
	})
}

func jsonOK(w http.ResponseWriter, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

func jsonErr(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": message,
	})
}
