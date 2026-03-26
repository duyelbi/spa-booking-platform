package config

import (
	"net"
	"net/url"
	"os"
	"regexp"
	"time"
)

type Config struct {
	AppEnv      string
	HTTPAddr    string
	DatabaseURL string
	RedisURL    string
	JWTSecret   string

	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration
	JWT2FATempTTL time.Duration
	OAuthStateTTL time.Duration
	TOTPIssuer    string

	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	OAuthRedirectURL        string
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// defaultDatabaseURL builds a DSN from POSTGRES_* when DATABASE_URL is unset.
// Matches typical local docker-compose published port (POSTGRES_PORT default 5433).
func defaultDatabaseURL() string {
	host := envOr("POSTGRES_HOST", "127.0.0.1")
	port := envOr("POSTGRES_PORT", "5433")
	user := envOr("POSTGRES_USER", "spa")
	pass := os.Getenv("POSTGRES_PASSWORD")
	db := envOr("POSTGRES_DB", "spa_booking")

	u := &url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + url.PathEscape(db),
	}
	if pass != "" {
		u.User = url.UserPassword(user, pass)
	} else {
		u.User = url.User(user)
	}
	q := url.Values{}
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String()
}

// envDefaultSyntax matches bash-style ${VAR:-default} for values loaded from .env.
// os.ExpandEnv only supports ${VAR} and $VAR.
var envDefaultSyntax = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*):-([^}]*)\}`)

func expandEnvDefaults(s string) string {
	for envDefaultSyntax.MatchString(s) {
		s = envDefaultSyntax.ReplaceAllStringFunc(s, func(match string) string {
			sub := envDefaultSyntax.FindStringSubmatch(match)
			if len(sub) != 3 {
				return match
			}
			name, defVal := sub[1], sub[2]
			if v := os.Getenv(name); v != "" {
				return v
			}
			return defVal
		})
	}
	return s
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
	v = expandEnvDefaults(v)
	return os.ExpandEnv(v)
}
