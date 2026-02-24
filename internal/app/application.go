package app

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"golan-project/internal/config"
	"golan-project/internal/controller"
	"golan-project/internal/middleware"
	"golan-project/internal/migrations"
	"golan-project/internal/platform/db"
	"golan-project/internal/platform/redisx"

	"github.com/redis/go-redis/v9"
)

type App struct {
	cfg        config.Config
	db         *sql.DB
	redis      *redis.Client
	controller *controller.Controller
}

func New(cfg config.Config) (*App, error) {
	database, err := db.Open(cfg.DBURL)
	if err != nil {
		return nil, err
	}
	cache, err := redisx.Open(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	app := &App{cfg: cfg, db: database, redis: cache}
	app.controller = controller.New(cfg, database, cache)
	if cfg.EnableAutoMigration {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := migrations.Run(ctx, database, filepath.Join("internal", "migrations", "sql")); err != nil {
			return nil, err
		}
	}
	return app, nil
}

func (a *App) Close() {
	if a.redis != nil {
		_ = a.redis.Close()
	}
	if a.db != nil {
		_ = a.db.Close()
	}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	a.registerRoutes(mux)
	return middleware.WithCommonHeaders(mux)
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
	app.controller.StartBackgroundWorkers(ctx)
	log.Printf("Witzo Go API listening on :%s", cfg.Port)
	return http.ListenAndServe(":"+cfg.Port, app.Handler())
}
