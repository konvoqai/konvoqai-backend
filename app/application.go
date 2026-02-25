package app

import (
	"context"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

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
	cfg   config.Config
	store *store.Store
	ctrl  *controller.Controller
}

func New(cfg config.Config) (*App, error) {
	database, err := db.Open(cfg.DBURL)
	if err != nil {
		return nil, err
	}
	cache, err := rediscache.Open(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	s := store.New(database, cache)
	app := &App{cfg: cfg, store: s}
	app.ctrl = controller.New(cfg, s.DB, s.Redis)
	if cfg.EnableAutoMigration {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := migrations.Run(ctx, s.DB, filepath.Join("migrations", "sql")); err != nil {
			return nil, err
		}
	}
	return app, nil
}

func (a *App) Close() {
	a.store.Close()
}

// auth wraps a controller handler with JWT + CSRF authentication.
func (a *App) auth(h func(http.ResponseWriter, *http.Request, controller.TokenClaims, controller.UserRecord)) http.HandlerFunc {
	return middleware.WithAuth(a.ctrl.AuthenticateUser, a.ctrl.RequireCSRF, utils.JSONErr, h)
}

// admin wraps a handler with admin JWT validation.
func (a *App) admin(h http.HandlerFunc) http.HandlerFunc {
	return middleware.WithAdmin(a.ctrl.ValidateAdminRequest, utils.JSONErr, h)
}

func (a *App) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return middleware.WithCommonHeaders(next, a.cfg.CORSAllowedOrigins)
	})
	a.registerRoutes(r)
	return r
}

func Run() error {
	cfg := config.Load()
	app, err := New(cfg)
	if err != nil {
		return err
	}
	defer app.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.ctrl.StartBackgroundWorkers(ctx)
	log.Printf("Konvoq Go API listening on :%s", cfg.Port)
	return http.ListenAndServe(":"+cfg.Port, app.Handler())
}
