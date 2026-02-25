package controller

import (
	"log/slog"
	"net/http"

	"konvoq-backend/utils"
)

func (c *Controller) requestLogger(r *http.Request) *slog.Logger {
	logger := c.logger
	if logger == nil {
		logger = slog.Default()
	}
	if r == nil {
		return logger
	}
	requestID := utils.RequestID(r.Context())
	if requestID == "" {
		return logger
	}
	return logger.With("request_id", requestID)
}

func (c *Controller) logRequestError(r *http.Request, message string, err error, attrs ...any) {
	if err == nil {
		return
	}
	attrs = append(attrs, "error", err)
	c.requestLogger(r).Error(message, attrs...)
}

func (c *Controller) logRequestWarn(r *http.Request, message string, err error, attrs ...any) {
	if err == nil {
		return
	}
	attrs = append(attrs, "error", err)
	c.requestLogger(r).Warn(message, attrs...)
}
