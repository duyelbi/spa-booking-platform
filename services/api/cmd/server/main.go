// Spa Booking Platform REST API.
// @title           Spa Booking API
// @version         1.0
// @description     REST API for branches, services, bookings, and auth (POST /api/v1/auth/register, /api/v1/auth/login). WebSocket live updates: GET /ws/live (not in OpenAPI).
//
// @termsOfService  http://swagger.io/terms/
//
// @contact.name   API Support
// @contact.email  support@example.com
//
// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT
//
// @host      localhost:8080
// @BasePath  /
// @schemes   http https
//
// @tag.name        auth
// @tag.description Register, login, refresh, logout, Google OAuth, 2FA, me
//
// @tag.name        bookings
// @tag.description Create and list bookings
//
// @tag.name        branches
// @tag.description Spa branches
//
// @tag.name        services
// @tag.description Services catalog
//
// @tag.name        system
// @tag.description Health and ops
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "spa-booking/services/api/docs"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"spa-booking/services/api/internal/config"
	"spa-booking/services/api/internal/db"
	"spa-booking/services/api/internal/handler"
	"spa-booking/services/api/internal/realtime"
	"spa-booking/services/api/internal/redisstore"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	config.LoadEnvFiles()
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	rdb, err := redisstore.Connect(cfg.RedisURL)
	if err != nil {
		log.Error("redis", "err", err)
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()

	hub := realtime.NewHub(log)
	go realtime.StartRedisSubscriber(ctx, rdb, hub, log)

	deps := handler.Deps{
		Log:           log,
		Pool:          pool,
		RDB:           rdb,
		Hub:           hub,
		JWTSecret:     []byte(cfg.JWTSecret),
		AccessTTL:     cfg.JWTAccessTTL,
		RefreshTTL:    cfg.JWTRefreshTTL,
		Temp2FATTL:    cfg.JWT2FATempTTL,
		OAuthStateTTL: cfg.OAuthStateTTL,
		TOTPIssuer:    cfg.TOTPIssuer,
	}
	if cfg.GoogleOAuthClientID != "" && cfg.GoogleOAuthClientSecret != "" && cfg.OAuthRedirectURL != "" {
		deps.GoogleOAuth = &oauth2.Config{
			ClientID:     cfg.GoogleOAuthClientID,
			ClientSecret: cfg.GoogleOAuthClientSecret,
			RedirectURL:  cfg.OAuthRedirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		}
	}

	r := handler.NewRouter(deps)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "err", err)
	}
}
