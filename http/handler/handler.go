package handler

import (
	"database/sql"
	"net/http"
	"time"

	"konvoq-backend/config"
	"konvoq-backend/http/handler/auth"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	cfg   config.Config
	db    *sql.DB
	redis *redis.Client

	Auth *auth.Handler
}

func New(cfg config.Config, db *sql.DB, redis *redis.Client) *Handler {
	c := &Handler{
		cfg:   cfg,
		db:    db,
		redis: redis,
	}
	c.Auth = auth.New()
	return c
}

type TokenClaims struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	SessionID int64  `json:"session_id"`
	Type      string `json:"type"`
	jwt.RegisteredClaims
}

type UserRecord struct {
	ID                    string
	Email                 string
	IsVerified            bool
	PlanType              string
	ConversationsUsed     int
	ConversationsLimit    sql.NullInt64
	PlanResetDate         time.Time
	LoginCount            int
	FullName              sql.NullString
	CompanyName           sql.NullString
	PhoneNumber           sql.NullString
	Country               sql.NullString
	JobTitle              sql.NullString
	Industry              sql.NullString
	CompanyWebsite        sql.NullString
	ProfileCompleted      bool
	ProfilePromptRequired sql.NullTime
	ProfileCompletedAt    sql.NullTime
}

func (h *Handler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	h.Auth.Google(w, r)
}

func (h *Handler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	h.Auth.GoogleCallback(w, r)
}
