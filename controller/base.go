package controller

import (
	"database/sql"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"konvoq-backend/controller/auth"
	"konvoq-backend/utils"
	"konvoq-backend/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// Controller holds all dependencies for request handlers.
type Controller struct {
	cfg   config.Config
	db    *sql.DB
	redis *redis.Client
	Auth  *auth.Handler
}

func New(cfg config.Config, db *sql.DB, redisClient *redis.Client) *Controller {
	c := &Controller{
		cfg:   cfg,
		db:    db,
		redis: redisClient,
	}
	c.Auth = auth.New()
	return c
}

// TokenClaims holds JWT payload for authenticated users.
type TokenClaims struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	SessionID int64  `json:"session_id"`
	Type      string `json:"type"`
	jwt.RegisteredClaims
}

// UserRecord holds the full user row loaded on each authenticated request.
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

func (c *Controller) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	c.Auth.Google(w, r)
}

func (c *Controller) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	c.Auth.GoogleCallback(w, r)
}

func scanUser(row *sql.Row) (UserRecord, error) {
	var u UserRecord
	err := row.Scan(
		&u.ID, &u.Email, &u.IsVerified, &u.PlanType,
		&u.ConversationsUsed, &u.ConversationsLimit, &u.PlanResetDate,
		&u.LoginCount, &u.FullName, &u.CompanyName, &u.PhoneNumber,
		&u.Country, &u.JobTitle, &u.Industry, &u.CompanyWebsite,
		&u.ProfileCompleted, &u.ProfilePromptRequired, &u.ProfileCompletedAt,
	)
	return u, err
}

func userResponse(u UserRecord, sessionID int64) map[string]interface{} {
	return map[string]interface{}{
		"id":                        u.ID,
		"email":                     u.Email,
		"isVerified":                u.IsVerified,
		"plan_type":                 u.PlanType,
		"sessionId":                 sessionID,
		"loginCount":                u.LoginCount,
		"fullName":                  utils.NullString(u.FullName),
		"companyName":               utils.NullString(u.CompanyName),
		"phoneNumber":               utils.NullString(u.PhoneNumber),
		"country":                   utils.NullString(u.Country),
		"jobTitle":                  utils.NullString(u.JobTitle),
		"industry":                  utils.NullString(u.Industry),
		"companyWebsite":            utils.NullString(u.CompanyWebsite),
		"profileCompleted":          u.ProfileCompleted,
		"requiresProfileCompletion": !u.ProfileCompleted && u.LoginCount > 3,
		"profilePromptRequiredAt":   utils.NullTime(u.ProfilePromptRequired),
		"profileCompletedAt":        utils.NullTime(u.ProfileCompletedAt),
	}
}

func readUploadedFile(r *http.Request, field string) (string, int64, string, error) {
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		return "", 0, "", errors.New("invalid multipart form")
	}
	files := r.MultipartForm.File[field]
	if len(files) == 0 {
		return "", 0, "", fmt.Errorf("missing '%s' file", field)
	}
	h := files[0]
	return h.Filename, h.Size, h.Header.Get("Content-Type"), nil
}

func docFromHeader(h *multipart.FileHeader) (string, int64, string) {
	return h.Filename, h.Size, h.Header.Get("Content-Type")
}

func coalesce(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}
