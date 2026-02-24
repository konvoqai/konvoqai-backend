package app

import (
	"net/http"

	"konvoq-backend/http/middleware"
	httputils "konvoq-backend/http/utils"
)

// registerRoutes attaches all HTTP routes to the mux using exported HTTP handlers.
func (a *App) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", a.handler.Health)
	mux.HandleFunc("GET /health/detailed", a.handler.HealthDetailed)
	mux.HandleFunc("GET /ready", a.handler.Ready)
	mux.HandleFunc("GET /live", a.handler.Live)
	mux.HandleFunc("GET /metrics", a.handler.Metrics)

	mux.HandleFunc("GET /api/auth/csrf-token", a.handler.GetCSRFToken)
	mux.HandleFunc("POST /api/auth/request-code", a.handler.RequestCode)
	mux.HandleFunc("POST /api/auth/verify", a.handler.VerifyCode)
	mux.HandleFunc("POST /api/auth/refresh", a.handler.RefreshToken)
	mux.HandleFunc("POST /api/auth/logout", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.Logout))
	mux.HandleFunc("GET /api/auth/me", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.Me))
	mux.HandleFunc("GET /api/auth/validate", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.ValidateSession))
	mux.HandleFunc("GET /api/auth/profile", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.ProfileStatus))
	mux.HandleFunc("PUT /api/auth/profile", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.UpdateProfile))
	mux.HandleFunc("GET /api/auth/sessions", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.GetSessions))
	mux.HandleFunc("DELETE /api/auth/sessions/{sessionId}", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.RevokeSession))
	mux.HandleFunc("POST /api/auth/sessions/revoke-all", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.RevokeAllOtherSessions))
	mux.HandleFunc("POST /api/auth/logout-all", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.LogoutAll))
	mux.HandleFunc("GET /api/auth/usage", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.GetUsage))
	mux.HandleFunc("POST /api/auth/usage/check", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.GetUsage))
	mux.HandleFunc("GET /api/auth/overview/analytics", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.Overview))

	mux.HandleFunc("POST /api/auth/scraper/scrape", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.Scrape))
	mux.HandleFunc("POST /api/auth/scraper/query", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.QueryDocuments))
	mux.HandleFunc("DELETE /api/auth/scraper/delete", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.DeleteSource))
	mux.HandleFunc("DELETE /api/auth/scraper/delete-page", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.DeleteSource))
	mux.HandleFunc("DELETE /api/auth/scraper/delete-all", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.DeleteAllSources))
	mux.HandleFunc("POST /api/auth/scraper/retrain", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.Scrape))
	mux.HandleFunc("GET /api/auth/scraper/stats", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.SourceStats))
	mux.HandleFunc("GET /api/auth/scraper/sources", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.GetSources))

	mux.HandleFunc("POST /api/auth/chat", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.Chat))
	mux.HandleFunc("GET /api/auth/chat/sessions", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.ChatSessions))
	mux.HandleFunc("GET /api/auth/chat/session/{sessionId}", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.ChatSession))
	mux.HandleFunc("DELETE /api/auth/chat/session/{sessionId}", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.ClearChatSession))
	mux.HandleFunc("POST /api/auth/chat/clear-user-sessions", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.ClearUserSessions))
	mux.HandleFunc("GET /api/auth/google", a.handler.GoogleLogin)
	mux.HandleFunc("GET /api/auth/google/callback", a.handler.GoogleCallback)
	mux.HandleFunc("POST /api/auth/google/verify", a.handler.VerifyGoogle)

	mux.HandleFunc("POST /api/auth/documents/upload", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.UploadDocument))
	mux.HandleFunc("POST /api/auth/documents/upload-multiple", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.UploadMultipleDocuments))

	mux.HandleFunc("POST /api/auth/widget/create", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.CreateWidget))
	mux.HandleFunc("GET /api/auth/widget/key", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.GetWidget))
	mux.HandleFunc("PUT /api/auth/widget/update", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.UpdateWidget))
	mux.HandleFunc("POST /api/auth/widget/regenerate", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.RegenerateWidget))
	mux.HandleFunc("DELETE /api/auth/widget/delete", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.DeleteWidget))
	mux.HandleFunc("GET /api/auth/widget/analytics", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.WidgetAnalytics))

	mux.HandleFunc("GET /api/auth/leads", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.ListLeads))
	mux.HandleFunc("GET /api/auth/leads/{id}", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.GetLead))
	mux.HandleFunc("PATCH /api/auth/leads/{id}/status", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.UpdateLeadStatus))
	mux.HandleFunc("DELETE /api/auth/leads/{id}", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.DeleteLead))
	mux.HandleFunc("GET /api/auth/leads/webhook", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.GetLeadWebhook))
	mux.HandleFunc("PUT /api/auth/leads/webhook", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.UpsertLeadWebhook))
	mux.HandleFunc("POST /api/auth/leads/webhook/test", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.LeadWebhookTest))
	mux.HandleFunc("GET /api/auth/leads/webhook/events", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.ListWebhookEvents))
	mux.HandleFunc("POST /api/auth/leads/webhook/events/{eventId}/retry", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.RetryWebhookEvent))

	mux.HandleFunc("GET /api/auth/feedback", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.ListFeedback))
	mux.HandleFunc("POST /api/auth/feedback", middleware.WithAuth(a.handler.AuthenticateUser, a.handler.RequireCSRF, httputils.JSONErr, a.handler.CreateFeedback))

	mux.HandleFunc("POST /api/admin/login", a.handler.AdminLogin)
	mux.HandleFunc("GET /api/admin/dashboard", middleware.WithAdmin(a.handler.ValidateAdminRequest, httputils.JSONErr, a.handler.AdminDashboard))
	mux.HandleFunc("GET /api/admin/insights", middleware.WithAdmin(a.handler.ValidateAdminRequest, httputils.JSONErr, a.handler.AdminInsights))
	mux.HandleFunc("GET /api/admin/users", middleware.WithAdmin(a.handler.ValidateAdminRequest, httputils.JSONErr, a.handler.AdminUsers))
	mux.HandleFunc("POST /api/admin/actions/reset-usage", middleware.WithAdmin(a.handler.ValidateAdminRequest, httputils.JSONErr, a.handler.AdminResetUsage))
	mux.HandleFunc("POST /api/admin/actions/force-logout", middleware.WithAdmin(a.handler.ValidateAdminRequest, httputils.JSONErr, a.handler.AdminForceLogout))
	mux.HandleFunc("POST /api/admin/actions/set-plan", middleware.WithAdmin(a.handler.ValidateAdminRequest, httputils.JSONErr, a.handler.AdminSetPlan))

	mux.HandleFunc("GET /api/v1/widget/config/{widgetKey}", a.handler.PublicWidgetConfig)
	mux.HandleFunc("POST /api/v1/webhook", a.handler.PublicWebhook)
	mux.HandleFunc("GET /api/v1/widget/embed.js", a.handler.EmbedJS)
	mux.HandleFunc("GET /api/v1/embed/", a.handler.EmbedForWidget)
	mux.HandleFunc("POST /api/v1/widget/contact", a.handler.PublicContact)
	mux.HandleFunc("POST /api/v1/widget/rating", a.handler.PublicRating)
}
