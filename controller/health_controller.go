package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"konvoq-backend/utils"
)

var startedAt = time.Now()

func (c *Controller) Health(w http.ResponseWriter, _ *http.Request) {
	utils.JSONOK(w, map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"uptime":    time.Since(startedAt).Seconds(),
	})
}

func (c *Controller) HealthDetailed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	dbStatus := "healthy"
	if err := c.db.PingContext(ctx); err != nil {
		dbStatus = "unhealthy"
		c.logRequestWarn(r, "health check database ping failed", err)
	}
	redisStatus := "healthy"
	if err := c.redis.Ping(ctx).Err(); err != nil {
		redisStatus = "unhealthy"
		c.logRequestWarn(r, "health check redis ping failed", err)
	}
	status := "healthy"
	if dbStatus != "healthy" || redisStatus != "healthy" {
		status = "degraded"
	}
	code := http.StatusOK
	if status != "healthy" {
		code = http.StatusServiceUnavailable
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       status,
		"dependencies": map[string]string{"database": dbStatus, "redis": redisStatus},
		"timestamp":    time.Now().UTC(),
	})
}

func (c *Controller) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := c.db.PingContext(ctx); err != nil {
		c.logRequestWarn(r, "readiness database ping failed", err)
		utils.JSONErr(w, http.StatusServiceUnavailable, "not ready")
		return
	}
	utils.JSONOK(w, map[string]interface{}{"status": "ready", "timestamp": time.Now().UTC()})
}

func (c *Controller) Live(w http.ResponseWriter, _ *http.Request) {
	utils.JSONOK(w, map[string]interface{}{"status": "alive", "timestamp": time.Now().UTC()})
}

func (c *Controller) Metrics(w http.ResponseWriter, r *http.Request) {
	var users, sessions int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		c.logRequestWarn(r, "metrics users count failed", err)
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE is_revoked=FALSE`).Scan(&sessions); err != nil {
		c.logRequestWarn(r, "metrics sessions count failed", err)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(fmt.Sprintf("witzo_users_total %d\nwitzo_sessions_total %d\n", users, sessions)))
}
