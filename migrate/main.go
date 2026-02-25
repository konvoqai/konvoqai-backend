package main

import (
	"context"
	"log/slog"
	"net"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"konvoq-backend/envx"
	"konvoq-backend/migrations"
	"konvoq-backend/platform/db"
	applog "konvoq-backend/platform/logger"
)

func main() {
	if err := envx.LoadDotEnvOverrideIfPresent(".env"); err != nil {
		slog.Error("failed to load .env", "error", err)
		os.Exit(1)
	}

	logger := loggerFromEnv()
	slog.SetDefault(logger)

	dbURL := databaseURLFromEnv()
	logger.Info("connecting database", "database_url", redactDatabaseURL(dbURL))

	database, err := db.Open(dbURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := migrations.Run(ctx, database, filepath.Join("migrations", "sql")); err != nil {
		logger.Error("database migrations failed", "error", err)
		os.Exit(1)
	}

	logger.Info("database migrations completed")
}

func databaseURLFromEnv() string {
	dbHost := getenv("DB_HOST", "localhost")
	dbPort := getenvInt("DB_PORT", 5432)
	dbName := getenv("DB_NAME", "auth_db")
	dbUser := getenv("DB_USER", "postgres")
	dbPass := getenv("DB_PASSWORD", "postgres")

	if hasExplicitDBParts() {
		return buildDatabaseURL(dbHost, dbPort, dbName, dbUser, dbPass)
	}

	if v := strings.TrimSpace(os.Getenv("DATABASE_URL")); v != "" {
		return applyDefaultSSLMode(strings.Trim(v, "\""))
	}

	return buildDatabaseURL(dbHost, dbPort, dbName, dbUser, dbPass)
}

func getenv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return strings.Trim(v, "\"")
}

func getenvInt(key string, fallback int) int {
	v := getenv(key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getenvBool(key string, fallback bool) bool {
	v := strings.ToLower(getenv(key, ""))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func loggerFromEnv() *slog.Logger {
	env := strings.ToLower(getenv("GO_ENV", getenv("NODE_ENV", "development")))
	format := getenv("LOG_FORMAT", "text")
	defaultLogColor := true
	return applog.New(applog.Config{
		Service:     getenv("SERVICE_NAME", "konvoq-migrate"),
		Environment: env,
		Level:       getenv("LOG_LEVEL", "info"),
		Format:      format,
		AddSource:   getenvBool("LOG_ADD_SOURCE", false),
		Color:       getenvBool("LOG_COLOR", defaultLogColor),
	})
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

func redactDatabaseURL(dbURL string) string {
	u, err := neturl.Parse(dbURL)
	if err != nil {
		return dbURL
	}
	if u.User != nil {
		username := u.User.Username()
		u.User = neturl.UserPassword(username, "****")
	}
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
