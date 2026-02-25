package app

import "net/http"

// registerRoutes attaches all HTTP routes to the mux.
func (a *App) registerRoutes(mux *http.ServeMux) {
	// Health / readiness
	mux.HandleFunc("GET /health", a.ctrl.Health)
	mux.HandleFunc("GET /health/detailed", a.ctrl.HealthDetailed)
	mux.HandleFunc("GET /ready", a.ctrl.Ready)
	mux.HandleFunc("GET /live", a.ctrl.Live)
	mux.HandleFunc("GET /metrics", a.ctrl.Metrics)

	// Auth (public)
	mux.HandleFunc("GET /api/auth/csrf-token", a.ctrl.GetCSRFToken)
	mux.HandleFunc("POST /api/auth/request-code", a.ctrl.RequestCode)
	mux.HandleFunc("POST /api/auth/verify", a.ctrl.VerifyCode)
	mux.HandleFunc("POST /api/auth/refresh", a.ctrl.RefreshToken)
	mux.HandleFunc("GET /api/auth/google", a.ctrl.GoogleLogin)
	mux.HandleFunc("GET /api/auth/google/callback", a.ctrl.GoogleCallback)
	mux.HandleFunc("POST /api/auth/google/verify", a.ctrl.VerifyGoogle)

	// Auth (protected)
	mux.HandleFunc("POST /api/auth/logout", a.auth(a.ctrl.Logout))
	mux.HandleFunc("GET /api/auth/me", a.auth(a.ctrl.Me))
	mux.HandleFunc("GET /api/auth/validate", a.auth(a.ctrl.ValidateSession))
	mux.HandleFunc("GET /api/auth/profile", a.auth(a.ctrl.ProfileStatus))
	mux.HandleFunc("PUT /api/auth/profile", a.auth(a.ctrl.UpdateProfile))
	mux.HandleFunc("GET /api/auth/sessions", a.auth(a.ctrl.GetSessions))
	mux.HandleFunc("DELETE /api/auth/sessions/{sessionId}", a.auth(a.ctrl.RevokeSession))
	mux.HandleFunc("POST /api/auth/sessions/revoke-all", a.auth(a.ctrl.RevokeAllOtherSessions))
	mux.HandleFunc("POST /api/auth/logout-all", a.auth(a.ctrl.LogoutAll))

	// Usage & analytics
	mux.HandleFunc("GET /api/auth/usage", a.auth(a.ctrl.GetUsage))
	mux.HandleFunc("POST /api/auth/usage/check", a.auth(a.ctrl.GetUsage))
	mux.HandleFunc("GET /api/auth/overview/analytics", a.auth(a.ctrl.Overview))

	// Scraper
	mux.HandleFunc("POST /api/auth/scraper/scrape", a.auth(a.ctrl.Scrape))
	mux.HandleFunc("POST /api/auth/scraper/query", a.auth(a.ctrl.QueryDocuments))
	mux.HandleFunc("DELETE /api/auth/scraper/delete", a.auth(a.ctrl.DeleteSource))
	mux.HandleFunc("DELETE /api/auth/scraper/delete-page", a.auth(a.ctrl.DeleteSource))
	mux.HandleFunc("DELETE /api/auth/scraper/delete-all", a.auth(a.ctrl.DeleteAllSources))
	mux.HandleFunc("POST /api/auth/scraper/retrain", a.auth(a.ctrl.Scrape))
	mux.HandleFunc("GET /api/auth/scraper/stats", a.auth(a.ctrl.SourceStats))
	mux.HandleFunc("GET /api/auth/scraper/sources", a.auth(a.ctrl.GetSources))

	// Chat
	mux.HandleFunc("POST /api/auth/chat", a.auth(a.ctrl.Chat))
	mux.HandleFunc("GET /api/auth/chat/sessions", a.auth(a.ctrl.ChatSessions))
	mux.HandleFunc("GET /api/auth/chat/session/{sessionId}", a.auth(a.ctrl.ChatSession))
	mux.HandleFunc("DELETE /api/auth/chat/session/{sessionId}", a.auth(a.ctrl.ClearChatSession))
	mux.HandleFunc("POST /api/auth/chat/clear-user-sessions", a.auth(a.ctrl.ClearUserSessions))

	// Documents
	mux.HandleFunc("POST /api/auth/documents/upload", a.auth(a.ctrl.UploadDocument))
	mux.HandleFunc("POST /api/auth/documents/upload-multiple", a.auth(a.ctrl.UploadMultipleDocuments))

	// Widget
	mux.HandleFunc("POST /api/auth/widget/create", a.auth(a.ctrl.CreateWidget))
	mux.HandleFunc("GET /api/auth/widget/key", a.auth(a.ctrl.GetWidget))
	mux.HandleFunc("PUT /api/auth/widget/update", a.auth(a.ctrl.UpdateWidget))
	mux.HandleFunc("POST /api/auth/widget/regenerate", a.auth(a.ctrl.RegenerateWidget))
	mux.HandleFunc("DELETE /api/auth/widget/delete", a.auth(a.ctrl.DeleteWidget))
	mux.HandleFunc("GET /api/auth/widget/analytics", a.auth(a.ctrl.WidgetAnalytics))

	// Leads
	mux.HandleFunc("GET /api/auth/leads", a.auth(a.ctrl.ListLeads))
	mux.HandleFunc("GET /api/auth/leads/{id}", a.auth(a.ctrl.GetLead))
	mux.HandleFunc("PATCH /api/auth/leads/{id}/status", a.auth(a.ctrl.UpdateLeadStatus))
	mux.HandleFunc("DELETE /api/auth/leads/{id}", a.auth(a.ctrl.DeleteLead))
	mux.HandleFunc("GET /api/auth/leads/webhook", a.auth(a.ctrl.GetLeadWebhook))
	mux.HandleFunc("PUT /api/auth/leads/webhook", a.auth(a.ctrl.UpsertLeadWebhook))
	mux.HandleFunc("POST /api/auth/leads/webhook/test", a.auth(a.ctrl.LeadWebhookTest))
	mux.HandleFunc("GET /api/auth/leads/webhook/events", a.auth(a.ctrl.ListWebhookEvents))
	mux.HandleFunc("POST /api/auth/leads/webhook/events/{eventId}/retry", a.auth(a.ctrl.RetryWebhookEvent))

	// Feedback
	mux.HandleFunc("GET /api/auth/feedback", a.auth(a.ctrl.ListFeedback))
	mux.HandleFunc("POST /api/auth/feedback", a.auth(a.ctrl.CreateFeedback))

	// Admin
	mux.HandleFunc("POST /api/admin/login", a.ctrl.AdminLogin)
	mux.HandleFunc("GET /api/admin/dashboard", a.admin(a.ctrl.AdminDashboard))
	mux.HandleFunc("GET /api/admin/insights", a.admin(a.ctrl.AdminInsights))
	mux.HandleFunc("GET /api/admin/users", a.admin(a.ctrl.AdminUsers))
	mux.HandleFunc("POST /api/admin/actions/reset-usage", a.admin(a.ctrl.AdminResetUsage))
	mux.HandleFunc("POST /api/admin/actions/force-logout", a.admin(a.ctrl.AdminForceLogout))
	mux.HandleFunc("POST /api/admin/actions/set-plan", a.admin(a.ctrl.AdminSetPlan))

	// Public widget API
	mux.HandleFunc("GET /api/v1/widget/config/{widgetKey}", a.ctrl.PublicWidgetConfig)
	mux.HandleFunc("POST /api/v1/webhook", a.ctrl.PublicWebhook)
	mux.HandleFunc("GET /api/v1/widget/embed.js", a.ctrl.EmbedJS)
	mux.HandleFunc("GET /api/v1/embed/", a.ctrl.EmbedForWidget)
	mux.HandleFunc("POST /api/v1/widget/contact", a.ctrl.PublicContact)
	mux.HandleFunc("POST /api/v1/widget/rating", a.ctrl.PublicRating)
}
