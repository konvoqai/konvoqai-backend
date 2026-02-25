package main

import (
	"log/slog"
	"os"

	"konvoq-backend/app"
	"konvoq-backend/config"
	applog "konvoq-backend/platform/logger"
)

func main() {
	cfg := config.Load()
	logger := applog.New(applog.Config{
		Service:     cfg.ServiceName,
		Environment: cfg.Environment,
		Level:       cfg.LogLevel,
		Format:      cfg.LogFormat,
		AddSource:   cfg.LogAddSource,
		Color:       cfg.LogColor,
	})
	slog.SetDefault(logger)

	if err := app.Run(cfg, logger); err != nil {
		logger.Error("application exited with error", "error", err)
		os.Exit(1)
	}
}
