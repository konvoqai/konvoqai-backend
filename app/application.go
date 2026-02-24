package app

import (
	"context"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"konvoq-backend/config"
	"konvoq-backend/http/handler"
	"konvoq-backend/http/middleware"
	"konvoq-backend/migrations"
	"konvoq-backend/platform/db"
	"konvoq-backend/platform/rediscache"
)

type App struct {
	cfg     config.Config
	store   *Store
	handler *handler.Handler
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
	store := NewStore(database, cache)
	app := &App{cfg: cfg, store: store}
	app.handler = handler.New(cfg, store.DB, store.Redis)
	if cfg.EnableAutoMigration {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := migrations.Run(ctx, store.DB, filepath.Join("migrations", "sql")); err != nil {
			return nil, err
		}
	}
	return app, nil
}

func (a *App) Close() {
	a.store.Close()
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	a.registerRoutes(mux)
	return middleware.WithCommonHeaders(mux, a.cfg.CORSAllowedOrigins)
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
	app.handler.StartBackgroundWorkers(ctx)
	log.Printf("Konvoq Go API listening on :%s", cfg.Port)
	return http.ListenAndServe(":"+cfg.Port, app.Handler())
}
