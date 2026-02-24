package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
)

type Config struct {
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

	AccessTokenMinutes  int
	RefreshTokenDays    int
	VerifyCodeMinutes   int
	MaxVerifyAttempts   int
	AdminEmail          string
	AdminPassword       string
	EnableAutoMigration bool

	OpenAIAPIKey string
	OpenAIModel  string

	PineconeAPIKey      string
	PineconeIndexName   string
	PineconeEnvironment string

	EmailHost     string
	EmailPort     int
	EmailUser     string
	EmailPassword string
	EmailFrom     string

	WebhookProcessIntervalSec int
	AnalyticsFlushIntervalSec int
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

func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "dev-secret"
	}
	return hex.EncodeToString(b)
}

func requireSecret(key string) string {
	v := getEnv(key, "")
	if v != "" {
		return v
	}
	if strings.ToLower(getEnv("NODE_ENV", "development")) == "production" {
		panic("missing required env: " + key)
	}
	return randomSecret()
}

func Load() Config {
	dbPort := getEnvInt("DB_PORT", 5432)
	dbHost := getEnv("DB_HOST", "localhost")
	dbUser := getEnv("DB_USER", "postgres")
	dbPass := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "auth_db")
	dbURL := getEnv("DATABASE_URL", "")
	if dbURL == "" {
		dbURL = "postgres://" + dbUser + ":" + dbPass + "@" + dbHost + ":" + strconv.Itoa(dbPort) + "/" + dbName + "?sslmode=disable"
	}

	redisHost := getEnv("REDIS_CACHE_HOST", getEnv("REDIS_HOST", "localhost"))
	redisPort := getEnvInt("REDIS_CACHE_PORT", getEnvInt("REDIS_PORT", 6379))

	return Config{
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

		AccessTokenMinutes:  getEnvInt("ACCESS_TOKEN_EXPIRY_MINUTES", 15),
		RefreshTokenDays:    getEnvInt("REFRESH_TOKEN_EXPIRY_DAYS", 7),
		VerifyCodeMinutes:   getEnvInt("VERIFICATION_CODE_EXPIRY_MINUTES", 10),
		MaxVerifyAttempts:   getEnvInt("MAX_VERIFICATION_ATTEMPTS", 5),
		AdminEmail:          getEnv("ADMIN_EMAIL", "admin@witzo.local"),
		AdminPassword:       getEnv("ADMIN_PASSWORD", "change-me-admin-password"),
		EnableAutoMigration: getEnvBool("AUTO_MIGRATE", true),

		OpenAIAPIKey: getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:  getEnv("OPENAI_MODEL", "gpt-4o-mini"),

		PineconeAPIKey:      getEnv("PINECONE_API_KEY", ""),
		PineconeIndexName:   getEnv("PINECONE_INDEX_NAME", ""),
		PineconeEnvironment: getEnv("PINECONE_ENVIRONMENT", ""),

		EmailHost:     getEnv("EMAIL_HOST", ""),
		EmailPort:     getEnvInt("EMAIL_PORT", 587),
		EmailUser:     getEnv("EMAIL_USER", ""),
		EmailPassword: getEnv("EMAIL_PASSWORD", ""),
		EmailFrom:     getEnv("EMAIL_FROM", getEnv("EMAIL_FROM_ADDRESS", "noreply@witzo.ai")),

		WebhookProcessIntervalSec: getEnvInt("WEBHOOK_PROCESS_INTERVAL_SEC", 30),
		AnalyticsFlushIntervalSec: getEnvInt("ANALYTICS_FLUSH_INTERVAL_SEC", 60),
	}
}
