package config

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"konvoq-backend/envx"
)

type Config struct {
	Environment  string
	IsProduction bool
	ServiceName  string

	Port   string
	DBURL  string
	DBHost string
	DBPort int
	DBName string
	DBUser string
	DBPass string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	JWTSecret        string
	JWTRefreshSecret string
	AdminJWTSecret   string
	CookieSecret     string

	AccessTokenMinutes     int
	RefreshTokenDays       int
	VerifyCodeMinutes      int
	MaxVerifyAttempts      int
	AdminTokenExpiryHours  int
	AdminBootstrapEmail    string
	AdminBootstrapPassword string
	AdminBootstrapRole     string
	EnableAutoMigration    bool

	OpenAIAPIKey string
	OpenAIModel  string

	PineconeAPIKey      string
	PineconeIndexName   string
	PineconeEnvironment string
	PineconeHost        string
	PineconeDimension   int

	EmailHost     string
	EmailPort     int
	EmailUser     string
	EmailPassword string
	EmailFrom     string

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	WebhookProcessIntervalSec int
	AnalyticsFlushIntervalSec int

	LogLevel     string
	LogFormat    string
	LogAddSource bool
	LogColor     bool

	CORSAllowedOrigins []string

	AuthIncludeDevCode   bool
	ExposeDetailedHealth bool
	ExposeMetrics        bool
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return strings.Trim(v, "\"")
}

func getEnvInt(key string, fallback int) int {
	v := getEnv(key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	v := strings.ToLower(getEnv(key, ""))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func getEnvList(key string, fallback []string) []string {
	v := strings.TrimSpace(getEnv(key, ""))
	if v == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

func normalizeEnvironment(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "prod":
		return "production"
	case "dev":
		return "development"
	case "stage":
		return "staging"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func isProductionEnvironment(raw string) bool {
	return normalizeEnvironment(raw) == "production"
}

func isBcryptHash(value string) bool {
	v := strings.TrimSpace(value)
	return strings.HasPrefix(v, "$2a$") || strings.HasPrefix(v, "$2b$") || strings.HasPrefix(v, "$2y$")
}

func isValidAdminRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "super_admin", "admin", "support", "readonly":
		return true
	default:
		return false
	}
}

func requireSecret(key string) string {
	v := getEnv(key, "")
	if v != "" {
		return v
	}
	env := normalizeEnvironment(getEnv("GO_ENV", getEnv("NODE_ENV", "development")))
	if isProductionEnvironment(env) {
		panic("missing required env: " + key)
	}
	return randomSecret()
}

func Load() Config {
	_ = envx.LoadDotEnvIfPresent(".env")

	dbPort := getEnvInt("DB_PORT", 5432)
	dbHost := getEnv("DB_HOST", "localhost")
	dbUser := getEnv("DB_USER", "postgres")
	dbPass := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "auth_db")
	dbURL := getEnv("DATABASE_URL", "")
	if hasExplicitDBParts() {
		dbURL = buildDatabaseURL(dbHost, dbPort, dbName, dbUser, dbPass)
	} else if dbURL != "" {
		dbURL = applyDefaultSSLMode(dbURL)
	} else {
		dbURL = buildDatabaseURL(dbHost, dbPort, dbName, dbUser, dbPass)
	}

	redisHost := getEnv("REDIS_CACHE_HOST", getEnv("REDIS_HOST", "localhost"))
	redisPort := getEnvInt("REDIS_CACHE_PORT", getEnvInt("REDIS_PORT", 6379))
	environment := normalizeEnvironment(getEnv("GO_ENV", getEnv("NODE_ENV", "development")))
	defaultLogFormat := "text"
	defaultLogColor := true
	defaultCORSOrigins := []string{
		"http://localhost:3000",
		"http://localhost:3001",
		"http://localhost:3050",
		"http://localhost:3051",
		"http://localhost:3052",
	}
	adminBootstrapEmail := strings.ToLower(strings.TrimSpace(getEnv("ADMIN_EMAIL", "")))
	adminBootstrapPassword := strings.TrimSpace(getEnv("ADMIN_PASSWORD", ""))
	adminBootstrapRole := strings.ToLower(strings.TrimSpace(getEnv("ADMIN_BOOTSTRAP_ROLE", "super_admin")))
	if !isValidAdminRole(adminBootstrapRole) {
		adminBootstrapRole = "super_admin"
	}
	isProd := isProductionEnvironment(environment)
	corsAllowedOrigins := getEnvList("CORS_ALLOWED_ORIGINS", defaultCORSOrigins)

	if isProd {
		if len(corsAllowedOrigins) == 0 {
			panic("CORS_ALLOWED_ORIGINS must be configured in production")
		}
		for _, origin := range corsAllowedOrigins {
			if strings.TrimSpace(origin) == "*" {
				panic("wildcard CORS is not allowed in production")
			}
		}
		if adminBootstrapEmail == "" && adminBootstrapPassword != "" {
			panic("ADMIN_EMAIL is required when ADMIN_PASSWORD is configured in production")
		}
		if adminBootstrapEmail != "" {
			if adminBootstrapPassword == "" {
				panic("ADMIN_PASSWORD is required when ADMIN_EMAIL is configured in production")
			}
			if !isBcryptHash(adminBootstrapPassword) {
				panic("ADMIN_PASSWORD must be a bcrypt hash in production when ADMIN_EMAIL is set")
			}
		}
	}

	return Config{
		Environment:  environment,
		IsProduction: isProd,
		ServiceName:  getEnv("SERVICE_NAME", "konvoq-backend"),

		Port:   getEnv("PORT", "8080"),
		DBURL:  dbURL,
		DBHost: dbHost,
		DBPort: dbPort,
		DBName: dbName,
		DBUser: dbUser,
		DBPass: dbPass,

		RedisAddr:     redisHost + ":" + strconv.Itoa(redisPort),
		RedisPassword: getEnv("REDIS_CACHE_PASSWORD", getEnv("REDIS_PASSWORD", "")),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		JWTSecret:        requireSecret("JWT_SECRET"),
		JWTRefreshSecret: requireSecret("JWT_REFRESH_SECRET"),
		AdminJWTSecret:   requireSecret("ADMIN_JWT_SECRET"),
		CookieSecret:     requireSecret("COOKIE_SECRET"),

		AccessTokenMinutes:     getEnvInt("ACCESS_TOKEN_EXPIRY_MINUTES", 15),
		RefreshTokenDays:       getEnvInt("REFRESH_TOKEN_EXPIRY_DAYS", 7),
		VerifyCodeMinutes:      getEnvInt("VERIFICATION_CODE_EXPIRY_MINUTES", 10),
		MaxVerifyAttempts:      getEnvInt("MAX_VERIFICATION_ATTEMPTS", 5),
		AdminTokenExpiryHours:  getEnvInt("ADMIN_TOKEN_EXPIRY_HOURS", 24),
		AdminBootstrapEmail:    adminBootstrapEmail,
		AdminBootstrapPassword: adminBootstrapPassword,
		AdminBootstrapRole:     adminBootstrapRole,
		EnableAutoMigration:    getEnvBool("AUTO_MIGRATE", false),

		OpenAIAPIKey: getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:  getEnv("OPENAI_MODEL", "gpt-4o-mini"),

		PineconeAPIKey:      getEnv("PINECONE_API_KEY", ""),
		PineconeIndexName:   getEnv("PINECONE_INDEX_NAME", ""),
		PineconeEnvironment: getEnv("PINECONE_ENVIRONMENT", ""),
		PineconeHost:        getEnv("PINECONE_HOST", ""),
		PineconeDimension:   getEnvInt("PINECONE_DIMENSION", 0),

		EmailHost:     getEnv("EMAIL_HOST", ""),
		EmailPort:     getEnvInt("EMAIL_PORT", 587),
		EmailUser:     getEnv("EMAIL_USER", ""),
		EmailPassword: getEnv("EMAIL_PASSWORD", ""),
		EmailFrom:     getEnv("EMAIL_FROM", getEnv("EMAIL_FROM_ADDRESS", "noreply@konvoq.ai")),

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", ""),

		WebhookProcessIntervalSec: getEnvInt("WEBHOOK_PROCESS_INTERVAL_SEC", 30),
		AnalyticsFlushIntervalSec: getEnvInt("ANALYTICS_FLUSH_INTERVAL_SEC", 60),

		LogLevel:     getEnv("LOG_LEVEL", "info"),
		LogFormat:    getEnv("LOG_FORMAT", defaultLogFormat),
		LogAddSource: getEnvBool("LOG_ADD_SOURCE", false),
		LogColor:     getEnvBool("LOG_COLOR", defaultLogColor),

		CORSAllowedOrigins:   corsAllowedOrigins,
		AuthIncludeDevCode:   getEnvBool("AUTH_INCLUDE_DEV_CODE", false),
		ExposeDetailedHealth: getEnvBool("EXPOSE_DETAILED_HEALTH", false),
		ExposeMetrics:        getEnvBool("EXPOSE_METRICS", false),
	}
}

func buildDatabaseURL(host string, port int, dbName, user, pass string) string {
	u := &neturl.URL{
		Scheme: "postgres",
		User:   neturl.UserPassword(user, pass),
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   "/" + dbName,
	}
	q := u.Query()
	if isLocalHost(host) {
		q.Set("sslmode", "disable")
	} else {
		q.Set("sslmode", "require")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func applyDefaultSSLMode(dbURL string) string {
	u, err := neturl.Parse(strings.TrimSpace(dbURL))
	if err != nil {
		return dbURL
	}
	q := u.Query()
	if q.Get("sslmode") != "" {
		return u.String()
	}
	if isLocalHost(u.Hostname()) {
		q.Set("sslmode", "disable")
	} else {
		q.Set("sslmode", "require")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func isLocalHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "" || h == "localhost" || h == "127.0.0.1" || h == "::1"
}

func hasExplicitDBParts() bool {
	return strings.TrimSpace(os.Getenv("DB_HOST")) != "" ||
		strings.TrimSpace(os.Getenv("DB_PORT")) != "" ||
		strings.TrimSpace(os.Getenv("DB_NAME")) != "" ||
		strings.TrimSpace(os.Getenv("DB_USER")) != "" ||
		strings.TrimSpace(os.Getenv("DB_PASSWORD")) != ""
}
