package app

import (
	"encoding/json"
	"net/http"

	"golan-project/internal/controller"
	"golan-project/internal/middleware"
)

// registerRoutes attaches all HTTP routes to the mux using exported controller handlers.
func (a *App) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", a.controller.Health)
	mux.HandleFunc("GET /health/detailed", a.controller.HealthDetailed)
	mux.HandleFunc("GET /ready", a.controller.Ready)
	mux.HandleFunc("GET /live", a.controller.Live)
	mux.HandleFunc("GET /metrics", a.controller.Metrics)

	mux.HandleFunc("GET /api/auth/csrf-token", a.controller.GetCSRFToken)
	mux.HandleFunc("POST /api/auth/request-code", a.controller.RequestCode)
	mux.HandleFunc("POST /api/auth/verify", a.controller.VerifyCode)
	mux.HandleFunc("POST /api/auth/refresh", a.controller.RefreshToken)
	mux.HandleFunc("POST /api/auth/logout", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.Logout))
	mux.HandleFunc("GET /api/auth/me", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.Me))
	mux.HandleFunc("GET /api/auth/validate", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.ValidateSession))
	mux.HandleFunc("GET /api/auth/profile", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.ProfileStatus))
	mux.HandleFunc("PUT /api/auth/profile", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.UpdateProfile))
	mux.HandleFunc("GET /api/auth/sessions", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.GetSessions))
	mux.HandleFunc("DELETE /api/auth/sessions/{sessionId}", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.RevokeSession))
	mux.HandleFunc("POST /api/auth/sessions/revoke-all", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.RevokeAllOtherSessions))
	mux.HandleFunc("POST /api/auth/logout-all", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.LogoutAll))
	mux.HandleFunc("GET /api/auth/usage", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.GetUsage))
	mux.HandleFunc("POST /api/auth/usage/check", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.GetUsage))
	mux.HandleFunc("GET /api/auth/overview/analytics", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.Overview))

	mux.HandleFunc("POST /api/auth/scraper/scrape", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.Scrape))
	mux.HandleFunc("POST /api/auth/scraper/query", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.QueryDocuments))
	mux.HandleFunc("DELETE /api/auth/scraper/delete", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.DeleteSource))
	mux.HandleFunc("DELETE /api/auth/scraper/delete-page", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.DeleteSource))
	mux.HandleFunc("DELETE /api/auth/scraper/delete-all", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.DeleteAllSources))
	mux.HandleFunc("POST /api/auth/scraper/retrain", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.Scrape))
	mux.HandleFunc("GET /api/auth/scraper/stats", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.SourceStats))
	mux.HandleFunc("GET /api/auth/scraper/sources", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.GetSources))

	mux.HandleFunc("POST /api/auth/chat", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.Chat))
	mux.HandleFunc("GET /api/auth/chat/sessions", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.ChatSessions))
	mux.HandleFunc("GET /api/auth/chat/session/{sessionId}", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.ChatSession))
	mux.HandleFunc("DELETE /api/auth/chat/session/{sessionId}", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.ClearChatSession))
	mux.HandleFunc("POST /api/auth/chat/clear-user-sessions", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.ClearUserSessions))
	mux.HandleFunc("GET /api/auth/google", a.controller.Auth.Google)
	mux.HandleFunc("GET /api/auth/google/callback", a.controller.Auth.GoogleCallback)
	mux.HandleFunc("POST /api/auth/google/verify", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, func(w http.ResponseWriter, r *http.Request, _ controller.TokenClaims, _ controller.UserRecord) {
		a.controller.VerifyGoogle(w, r)
	}))

	mux.HandleFunc("POST /api/auth/documents/upload", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.UploadDocument))
	mux.HandleFunc("POST /api/auth/documents/upload-multiple", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.UploadMultipleDocuments))

	mux.HandleFunc("POST /api/auth/widget/create", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.CreateWidget))
	mux.HandleFunc("GET /api/auth/widget/key", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.GetWidget))
	mux.HandleFunc("PUT /api/auth/widget/update", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.UpdateWidget))
	mux.HandleFunc("POST /api/auth/widget/regenerate", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.RegenerateWidget))
	mux.HandleFunc("DELETE /api/auth/widget/delete", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.DeleteWidget))
	mux.HandleFunc("GET /api/auth/widget/analytics", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.WidgetAnalytics))

	mux.HandleFunc("GET /api/auth/leads", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.ListLeads))
	mux.HandleFunc("GET /api/auth/leads/{id}", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.GetLead))
	mux.HandleFunc("PATCH /api/auth/leads/{id}/status", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.UpdateLeadStatus))
	mux.HandleFunc("DELETE /api/auth/leads/{id}", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.DeleteLead))
	mux.HandleFunc("GET /api/auth/leads/webhook", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.GetLeadWebhook))
	mux.HandleFunc("PUT /api/auth/leads/webhook", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.UpsertLeadWebhook))
	mux.HandleFunc("POST /api/auth/leads/webhook/test", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.LeadWebhookTest))
	mux.HandleFunc("GET /api/auth/leads/webhook/events", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.ListWebhookEvents))
	mux.HandleFunc("POST /api/auth/leads/webhook/events/{eventId}/retry", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.RetryWebhookEvent))

	mux.HandleFunc("GET /api/auth/feedback", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.ListFeedback))
	mux.HandleFunc("POST /api/auth/feedback", middleware.WithAuth(a.controller.AuthenticateUser, a.controller.RequireCSRF, jsonErr, a.controller.CreateFeedback))

	mux.HandleFunc("POST /api/admin/login", a.controller.AdminLogin)
	mux.HandleFunc("GET /api/admin/dashboard", middleware.WithAdmin(a.controller.ValidateAdminRequest, jsonErr, a.controller.AdminDashboard))
	mux.HandleFunc("GET /api/admin/insights", middleware.WithAdmin(a.controller.ValidateAdminRequest, jsonErr, a.controller.AdminInsights))
	mux.HandleFunc("GET /api/admin/users", middleware.WithAdmin(a.controller.ValidateAdminRequest, jsonErr, a.controller.AdminUsers))
	mux.HandleFunc("POST /api/admin/actions/reset-usage", middleware.WithAdmin(a.controller.ValidateAdminRequest, jsonErr, a.controller.AdminResetUsage))
	mux.HandleFunc("POST /api/admin/actions/force-logout", middleware.WithAdmin(a.controller.ValidateAdminRequest, jsonErr, a.controller.AdminForceLogout))
	mux.HandleFunc("POST /api/admin/actions/set-plan", middleware.WithAdmin(a.controller.ValidateAdminRequest, jsonErr, a.controller.AdminSetPlan))

	mux.HandleFunc("GET /api/v1/widget/config/{widgetKey}", a.controller.PublicWidgetConfig)
	mux.HandleFunc("POST /api/v1/webhook", a.controller.PublicWebhook)
	mux.HandleFunc("GET /api/v1/widget/embed.js", a.controller.EmbedJS)
	mux.HandleFunc("GET /api/v1/embed/", a.controller.EmbedForWidget)
	mux.HandleFunc("POST /api/v1/widget/contact", a.controller.PublicContact)
	mux.HandleFunc("POST /api/v1/widget/rating", a.controller.PublicRating)
}

func jsonErr(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": message})
}
