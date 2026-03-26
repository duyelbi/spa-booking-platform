// Command migrate applies embedded SQL migrations and exits (no HTTP server).
// Use: go run ./cmd/migrate from services/api, or docker compose --profile tools run --rm migrate.
package main

import (
	"context"
	"log"
	"os"

	"spa-booking/services/api/internal/config"
	"spa-booking/services/api/internal/db"
)

func main() {
	log.SetFlags(0)
	config.LoadEnvFiles()
	cfg := config.Load()
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("migrations applied OK")
	os.Exit(0)
}
