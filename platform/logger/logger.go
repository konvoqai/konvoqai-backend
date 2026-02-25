package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Config controls logger output format and level.
type Config struct {
	Service     string
	Environment string
	Level       string
	Format      string
	AddSource   bool
	Color       bool
	Output      io.Writer
}

func New(cfg Config) *slog.Logger {
	level := parseLevel(cfg.Level)
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}

	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	format := strings.ToLower(strings.TrimSpace(cfg.Format))
	var handler slog.Handler
	switch format {
	case "text", "console":
		handler = NewPrettyHandler(out, PrettyHandlerOptions{
			Level:     opts.Level,
			AddSource: opts.AddSource,
			Color:     cfg.Color,
		})
	default:
		handler = slog.NewJSONHandler(out, opts)
	}

	logger := slog.New(handler)
	attrs := make([]any, 0, 4)
	if svc := strings.TrimSpace(cfg.Service); svc != "" {
		attrs = append(attrs, "service", svc)
	}
	if env := strings.TrimSpace(cfg.Environment); env != "" {
		attrs = append(attrs, "env", env)
	}
	if len(attrs) > 0 {
		logger = logger.With(attrs...)
	}
	return logger
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
