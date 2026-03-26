package config

import (
	"net/url"
	"os"
	"time"
)

type Config struct {
	AppEnv      string
	HTTPAddr    string
	DatabaseURL string
	RedisURL    string
	JWTSecret   string

	JWTAccessTTL   time.Duration
	JWTRefreshTTL  time.Duration
	JWT2FATempTTL  time.Duration
	OAuthStateTTL  time.Duration
	TOTPIssuer     string

	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	OAuthRedirectURL        string
}

func defaultDatabaseURL() string {
	db := os.Getenv("POSTGRES_DB")
	if db == "" {
		db = "spa_booking"
	}
	return "postgres://127.0.0.1:5433/" + url.PathEscape(db) + "?sslmode=disable"
}

func Load() Config {
	return Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: getEnv("DATABASE_URL", defaultDatabaseURL()),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:   getEnv("JWT_SECRET", "dev-secret-change-me"),

		JWTAccessTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL: getEnvDuration("JWT_REFRESH_TTL", 168*time.Hour),
		JWT2FATempTTL: getEnvDuration("JWT_2FA_TEMP_TTL", 5*time.Minute),
		OAuthStateTTL: getEnvDuration("OAUTH_STATE_TTL", 10*time.Minute),
		TOTPIssuer:    getEnv("TOTP_ISSUER", "SpaBooking"),

		GoogleOAuthClientID:     getEnv("GOOGLE_OAUTH_CLIENT_ID", ""),
		GoogleOAuthClientSecret: getEnv("GOOGLE_OAUTH_CLIENT_SECRET", ""),
		OAuthRedirectURL:        getEnv("OAUTH_REDIRECT_URL", ""),
	}
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		v = fallback
	}
	return os.ExpandEnv(v)
}
