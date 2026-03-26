# Run from repository root (same directory as docker-compose.yml).
COMPOSE := docker compose

.PHONY: help up down logs migrate-up db-shell backup api-rebuild restore

help:
	@echo "Spa booking — Docker helpers"
	@echo ""
	@echo "  make up           - docker compose up -d --build (postgres, redis, api)"
	@echo "  make down         - docker compose down"
	@echo "  make logs         - follow all logs; one service: make logs s=api"
	@echo "  make migrate-up   - run SQL migrations only (profile tools)"
	@echo "  make db-shell     - psql into Postgres (uses .env user/db)"
	@echo "  make backup       - pg_dump -> backups/postgres_<db>_<timestamp>.sql.gz"
	@echo "  make restore FILE=path/to/dump.sql.gz  - restore from backup"
	@echo "  make api-rebuild  - rebuild and restart api service only"
	@echo ""

up:
	$(COMPOSE) up -d --build

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f $(s)

migrate-up:
	$(COMPOSE) --profile tools run --rm migrate

db-shell:
	@scripts/db-shell.sh

backup:
	@scripts/db-backup.sh

restore:
	@test -n "$(FILE)" || (echo 'Usage: make restore FILE=backups/postgres_....sql.gz' >&2; exit 1)
	@scripts/db-restore.sh "$(FILE)"

api-rebuild:
	@scripts/docker-rebuild-api.sh
