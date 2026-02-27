package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *App) registerRoutes(r chi.Router) {
	// Health / readiness
	r.Get("/health", a.ctrl.Health)
	r.Get("/health/detailed", a.ctrl.HealthDetailed)
	r.Get("/ready", a.ctrl.Ready)
	r.Get("/live", a.ctrl.Live)
	r.Get("/metrics", a.ctrl.Metrics)

	r.Route("/api/auth", a.mapAuthRoutes)
	r.Route("/api/me", a.mapMeRoutes)
	r.Route("/api/scraper", a.mapScraperRoutes)
	r.Route("/api/chat", a.mapChatRoutes)
	r.Route("/api/documents", a.mapDocumentRoutes)
	r.Route("/api/widget", a.mapWidgetRoutes)
	r.Route("/api/leads", a.mapLeadsRoutes)
	r.Route("/api/feedback", a.mapFeedbackRoutes)
	r.Route("/api/admin", a.mapAdminRoutes)
	r.Route("/api/v1", a.mapPublicV1Routes)

	// Shared widget bundle for embed + dashboard live preview.
	r.Get("/widget/preview", a.ctrl.WidgetPreviewPage)
	r.Handle("/widget/*", http.StripPrefix("/widget/", http.FileServer(http.Dir("public/widget"))))
}

// Auth — public (no token required) + session management (protected)
func (a *App) mapAuthRoutes(r chi.Router) {
	r.Get("/csrf-token", a.ctrl.GetCSRFToken)
	r.Post("/request-code", a.ctrl.RequestCode)
	r.Post("/verify", a.ctrl.VerifyCode)
	r.Post("/refresh", a.ctrl.RefreshToken)
	r.Get("/google", a.ctrl.GoogleLogin)
	r.Get("/google/callback", a.ctrl.GoogleCallback)
	r.Post("/google/verify", a.ctrl.VerifyGoogle)

	r.Post("/logout", a.auth(a.ctrl.Logout))
	r.Post("/logout-all", a.auth(a.ctrl.LogoutAll))
	r.Get("/sessions", a.auth(a.ctrl.GetSessions))
	r.Delete("/sessions/{id}", a.auth(a.ctrl.RevokeSession))
	r.Post("/sessions/revoke-all", a.auth(a.ctrl.RevokeAllOtherSessions))
}

// Me — current user profile & status
func (a *App) mapMeRoutes(r chi.Router) {
	r.Get("/", a.auth(a.ctrl.Me))
	r.Get("/validate", a.auth(a.ctrl.ValidateSession))
	r.Get("/profile", a.auth(a.ctrl.ProfileStatus))
	r.Put("/profile", a.auth(a.ctrl.UpdateProfile))
	r.Post("/onboarding/complete", a.auth(a.ctrl.CompleteOnboarding))
	r.Get("/usage", a.auth(a.ctrl.GetUsage))
	r.Get("/analytics", a.auth(a.ctrl.Overview))
}

// Scraper
func (a *App) mapScraperRoutes(r chi.Router) {
	r.Get("/sources", a.auth(a.ctrl.GetSources))
	r.Get("/stats", a.auth(a.ctrl.SourceStats))
	r.Get("/jobs/{id}", a.auth(a.ctrl.GetScrapeJob))
	r.Post("/sources", a.auth(a.ctrl.Scrape))
	r.Post("/retrain", a.auth(a.ctrl.Scrape))
	r.Post("/query", a.auth(a.ctrl.QueryDocuments))
	r.Delete("/sources", a.auth(a.ctrl.DeleteSource))
	r.Delete("/sources/page", a.auth(a.ctrl.DeleteSource))
	r.Delete("/sources/all", a.auth(a.ctrl.DeleteAllSources))
}

// Chat
func (a *App) mapChatRoutes(r chi.Router) {
	r.Post("/", a.auth(a.ctrl.Chat))
	r.Get("/sessions", a.auth(a.ctrl.ChatSessions))
	r.Get("/sessions/{id}", a.auth(a.ctrl.ChatSession))
	r.Delete("/sessions/{id}", a.auth(a.ctrl.ClearChatSession))
	r.Delete("/sessions", a.auth(a.ctrl.ClearUserSessions))
}

// Documents
func (a *App) mapDocumentRoutes(r chi.Router) {
	r.Get("/", a.auth(a.ctrl.ListDocuments))
	r.Post("/", a.auth(a.ctrl.UploadDocument))
	r.Post("/batch", a.auth(a.ctrl.UploadMultipleDocuments))
	r.Delete("/{id}", a.auth(a.ctrl.DeleteDocument))
}

// Widget
func (a *App) mapWidgetRoutes(r chi.Router) {
	r.Post("/", a.auth(a.ctrl.CreateWidget))
	r.Get("/", a.auth(a.ctrl.GetWidget))
	r.Put("/", a.auth(a.ctrl.UpdateWidget))
	r.Post("/regenerate", a.auth(a.ctrl.RegenerateWidget))
	r.Delete("/", a.auth(a.ctrl.DeleteWidget))
	r.Get("/analytics", a.auth(a.ctrl.WidgetAnalytics))
}

// Leads
func (a *App) mapLeadsRoutes(r chi.Router) {
	r.Get("/", a.auth(a.ctrl.ListLeads))
	r.Get("/webhook", a.auth(a.ctrl.GetLeadWebhook))
	r.Put("/webhook", a.auth(a.ctrl.UpsertLeadWebhook))
	r.Post("/webhook/test", a.auth(a.ctrl.LeadWebhookTest))
	r.Get("/webhook/events", a.auth(a.ctrl.ListWebhookEvents))
	r.Post("/webhook/events/{id}/retry", a.auth(a.ctrl.RetryWebhookEvent))
	r.Get("/{id}", a.auth(a.ctrl.GetLead))
	r.Patch("/{id}/status", a.auth(a.ctrl.UpdateLeadStatus))
	r.Delete("/{id}", a.auth(a.ctrl.DeleteLead))
}

// Feedback
func (a *App) mapFeedbackRoutes(r chi.Router) {
	r.Get("/", a.auth(a.ctrl.ListFeedback))
	r.Post("/", a.auth(a.ctrl.CreateFeedback))
}

// Admin
func (a *App) mapAdminRoutes(r chi.Router) {
	r.Post("/login", a.ctrl.AdminLogin)
	r.Get("/dashboard", a.admin(a.ctrl.AdminDashboard))
	r.Get("/insights", a.admin(a.ctrl.AdminInsights))
	r.Get("/users", a.admin(a.ctrl.AdminUsers))
	r.Post("/actions/reset-usage", a.admin(a.ctrl.AdminResetUsage))
	r.Post("/actions/force-logout", a.admin(a.ctrl.AdminForceLogout))
	r.Post("/actions/set-plan", a.admin(a.ctrl.AdminSetPlan))
}

// Public widget API (versioned, no auth)
func (a *App) mapPublicV1Routes(r chi.Router) {
	r.Get("/widget/config/{widgetKey}", a.ctrl.PublicWidgetConfig)
	r.Post("/webhook", a.ctrl.PublicWebhook)
	r.Get("/widget/embed.js", a.ctrl.EmbedJS)
	r.Get("/embed/{widgetKey}.js", a.ctrl.EmbedForWidget)
	r.Post("/widget/contact", a.ctrl.PublicContact)
	r.Post("/widget/rating", a.ctrl.PublicRating)
}
