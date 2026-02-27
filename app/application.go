package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"konvoq-backend/config"
	"konvoq-backend/controller"
	"konvoq-backend/middleware"
	"konvoq-backend/migrations"
	"konvoq-backend/platform/db"
	"konvoq-backend/platform/rediscache"
	"konvoq-backend/store"
	"konvoq-backend/utils"
)

type App struct {
	cfg    config.Config
	store  *store.Store
	ctrl   *controller.Controller
	logger *slog.Logger
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "app")

	logger.Info("opening database connection")
	database, err := db.Open(cfg.DBURL)
	if err != nil {
		logger.Error("failed to open database connection", "error", err)
		return nil, err
	}

	logger.Info("opening redis connection", "redis_addr", cfg.RedisAddr)
	cache, err := rediscache.Open(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Error("failed to open redis connection", "error", err)
		_ = database.Close()
		return nil, err
	}

	s := store.New(database, cache)
	app := &App{cfg: cfg, store: s, logger: logger}
	app.ctrl = controller.New(cfg, s.DB, s.Redis, logger)
	if cfg.EnableAutoMigration {
		logger.Info("running startup migrations", "dir", filepath.Join("migrations", "sql"))
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := migrations.Run(ctx, s.DB, filepath.Join("migrations", "sql")); err != nil {
			logger.Error("startup migrations failed", "error", err)
			return nil, err
		}
		logger.Info("startup migrations completed")
	}
	logger.Info("application dependencies initialized")
	return app, nil
}

func (a *App) Close() {
	a.store.Close()
}

// auth wraps a controller handler with JWT + CSRF authentication.
func (a *App) auth(h func(http.ResponseWriter, *http.Request, controller.TokenClaims, controller.UserRecord)) http.HandlerFunc {
	return middleware.WithAuth(a.ctrl.AuthenticateUser, a.ctrl.RequireCSRF, utils.JSONErr, h, a.logger)
}

// admin wraps a handler with admin JWT validation.
func (a *App) admin(h http.HandlerFunc) http.HandlerFunc {
	return middleware.WithAdmin(a.ctrl.ValidateAdminRequest, utils.JSONErr, h, a.logger)
}

func (a *App) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.WithRequestLogger(a.logger))
	r.Use(middleware.WithRecovery(a.logger))
	// Global rate limit: 100 requests per minute per IP
	r.Use(httprate.LimitByIP(100, time.Minute))
	r.Use(func(next http.Handler) http.Handler {
		return middleware.WithCommonHeaders(next, a.cfg.CORSAllowedOrigins)
	})
	a.registerRoutes(r)
	return r
}

func Run(cfg config.Config, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	app, err := New(cfg, logger)
	if err != nil {
		return err
	}
	defer app.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.ctrl.StartBackgroundWorkers(ctx)

	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      app.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("konvoq api listening", "address", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for OS shutdown signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		logger.Error("http server error", "error", err)
		cancel()
		return err
	case sig := <-quit:
		logger.Info("shutdown signal received", "signal", sig.String())
	}

	// Cancel background workers
	cancel()

	// Give in-flight requests up to 30 s to complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
		return err
	}
	logger.Info("server stopped gracefully")
	return nil
}
