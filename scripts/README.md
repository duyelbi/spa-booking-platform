# Scripts — Postgres migrate & backup (Docker)

## Makefile (repo root)

You can use Make instead of calling `./scripts/...` directly:

```bash
make help          # list targets
make up            # docker compose up -d --build
make down          # docker compose down
make logs          # logs -f (one service: make logs s=api)
make migrate-up
make db-shell      # psql (via scripts/db-shell.sh)
make backup
make api-rebuild   # rebuild api service only (runs scripts/docker-rebuild-api.sh)
make restore FILE=backups/postgres_....sql.gz
```

## Docker: rebuild API only

After changing Go code or the `api` service Dockerfile:

```bash
make api-rebuild
# or: ./scripts/docker-rebuild-api.sh
```

Runs `docker compose build api` then `up -d api`. Postgres/Redis images are not rebuilt; DB volumes are unchanged.

## Schema migrations (SQL)

- **Automatic:** When the `api` container starts, the server runs `db.Migrate()` (SQL under `services/api/internal/db/migrations/`).
- **Migrations only (no HTTP API):**

  ```bash
  docker compose --profile tools run --rm migrate
  ```

  Requires a healthy Postgres (`docker compose up -d postgres`). Uses the same internal `DATABASE_URL` as the `api` service.

- **On the host (no Docker API):**

  ```bash
  cd services/api && go run ./cmd/migrate
  ```

  Root `.env` must define `DATABASE_URL` or enough `POSTGRES_*` variables (see `internal/config`).

## Backup

```bash
chmod +x scripts/db-backup.sh   # once
./scripts/db-backup.sh
```

Writes a gzip file `backups/postgres_<db>_<timestamp>.sql.gz` (the `backups/` directory is `.gitignore`d).

Requires: `docker compose up -d postgres` and `.env` with `POSTGRES_USER`, `POSTGRES_DB`, `POSTGRES_PASSWORD`.

## Restore

```bash
chmod +x scripts/db-restore.sh
./scripts/db-restore.sh backups/postgres_spa_booking_YYYYMMDD_HHMMSS.sql.gz
```

Dumps from `db-backup.sh` use `--clean --if-exists`: restore may **drop and recreate** objects — run only when you intend to.

## Redis (optional)

Redis data lives in the `redisdata` volume. Quick backup while the stack is running:

```bash
docker compose exec redis redis-cli SAVE
docker run --rm -v spa-booking-platform_redisdata:/data -v "$(pwd)/backups:/out" alpine \
  cp /data/dump.rdb "/out/redis_$(date +%Y%m%d_%H%M%S).rdb"
```

Volume names may differ; check with `docker volume ls | grep redis`.
