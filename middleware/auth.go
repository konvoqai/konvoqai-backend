package middleware

import (
	"log/slog"
	"net/http"

	"konvoq-backend/utils"
)

func WithAuth[C any, U any](
	authenticate func(*http.Request) (C, U, error),
	requireCSRF func(*http.Request) error,
	onError func(http.ResponseWriter, int, string),
	next func(http.ResponseWriter, *http.Request, C, U),
	logger *slog.Logger,
) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	authLogger := logger.With("component", "auth")
	return func(w http.ResponseWriter, r *http.Request) {
		claims, user, err := authenticate(r)
		if err != nil {
			authLogger.Warn("request authentication failed",
				"request_id", utils.RequestID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"error", err,
			)
			onError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if err := requireCSRF(r); err != nil {
				authLogger.Warn("csrf validation failed",
					"request_id", utils.RequestID(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"error", err,
				)
				onError(w, http.StatusForbidden, err.Error())
				return
			}
		}
		next(w, r, claims, user)
	}
}

func WithAdmin(
	validate func(*http.Request) error,
	onError func(http.ResponseWriter, int, string),
	next http.HandlerFunc,
	logger *slog.Logger,
) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	authLogger := logger.With("component", "auth")
	return func(w http.ResponseWriter, r *http.Request) {
		if err := validate(r); err != nil {
			authLogger.Warn("admin request validation failed",
				"request_id", utils.RequestID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"error", err,
			)
			onError(w, http.StatusUnauthorized, err.Error())
			return
		}
		next(w, r)
	}
}
