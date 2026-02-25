package auth

import (
	"encoding/json"
	"net/http"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Google(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]interface{}{"success": true, "authUrl": "https://accounts.google.com/o/oauth2/auth"})
}

func (h *Handler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]interface{}{"success": true, "state": r.URL.Query().Get("state"), "code": r.URL.Query().Get("code")})
}

func jsonOK(w http.ResponseWriter, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}
