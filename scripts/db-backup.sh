#!/usr/bin/env bash
# Backup Postgres used by docker-compose (service: postgres).
# Run from repository root with stack up: docker compose up -d postgres
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
  echo "Missing .env — copy from .env.example and set POSTGRES_PASSWORD." >&2
  exit 1
fi

# shellcheck disable=SC1091
set -a && source .env && set +a

: "${POSTGRES_USER:=spa}"
: "${POSTGRES_DB:=spa_booking}"

mkdir -p backups
STAMP="$(date +%Y%m%d_%H%M%S)"
OUT="backups/postgres_${POSTGRES_DB}_${STAMP}.sql.gz"

docker compose exec -T postgres \
  pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  --no-owner --no-acl --clean --if-exists \
  | gzip >"$OUT"

echo "Backup written: $OUT"
