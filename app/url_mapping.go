package app

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
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
	r.Route("/api/projects", a.mapProjectRoutes)
	r.Route("/api/chatbots", a.mapChatbotRoutes)
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
	r.With(httprate.LimitByIP(20, 10*time.Minute)).Post("/request-code", a.ctrl.RequestCode)
	r.With(httprate.LimitByIP(30, 10*time.Minute)).Post("/verify", a.ctrl.VerifyCode)
	r.With(httprate.LimitByIP(60, 10*time.Minute)).Post("/refresh", a.ctrl.RefreshToken)
	r.Get("/google", a.ctrl.GoogleLogin)
	r.Get("/google/callback", a.ctrl.GoogleCallback)
	r.With(httprate.LimitByIP(30, 10*time.Minute)).Post("/google/verify", a.ctrl.VerifyGoogle)

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
	r.Get("/brand-extract", a.auth(a.ctrl.BrandExtract))
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

// Projects and chatbots (currently mapped to per-user widget resources)
func (a *App) mapProjectRoutes(r chi.Router) {
	r.Get("/", a.auth(a.ctrl.ListProjects))
}

func (a *App) mapChatbotRoutes(r chi.Router) {
	r.Get("/", a.auth(a.ctrl.ListChatbots))
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
	r.With(httprate.LimitByIP(10, 10*time.Minute)).Post("/login", a.ctrl.AdminLogin)
	r.Get("/dashboard", a.adminRoles(a.ctrl.AdminDashboard, "super_admin", "admin", "support", "readonly"))
	r.Get("/insights", a.adminRoles(a.ctrl.AdminInsights, "super_admin", "admin", "readonly"))
	r.Get("/users", a.adminRoles(a.ctrl.AdminUsers, "super_admin", "admin", "support", "readonly"))
	r.Post("/actions/reset-usage", a.adminRoles(a.ctrl.AdminResetUsage, "super_admin", "admin"))
	r.Post("/actions/force-logout", a.adminRoles(a.ctrl.AdminForceLogout, "super_admin", "admin"))
	r.Post("/actions/set-plan", a.adminRoles(a.ctrl.AdminSetPlan, "super_admin"))
}

// Public widget API (versioned, no auth)
func (a *App) mapPublicV1Routes(r chi.Router) {
	r.Get("/widget/config/preview", a.ctrl.PublicWidgetPreviewConfig)
	r.Get("/widget/config/{widgetKey}", a.ctrl.PublicWidgetConfig)
	r.With(httprate.LimitByIP(300, time.Minute)).Post("/webhook", a.ctrl.PublicWebhook)
	r.Get("/widget/embed.js", a.ctrl.EmbedJS)
	r.Get("/embed/{widgetKey}.js", a.ctrl.EmbedForWidget)
	r.With(httprate.LimitByIP(120, time.Minute)).Post("/widget/contact", a.ctrl.PublicContact)
	r.With(httprate.LimitByIP(240, time.Minute)).Post("/widget/rating", a.ctrl.PublicRating)
}
