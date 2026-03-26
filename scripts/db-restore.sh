#!/usr/bin/env bash
# Restore Postgres from a plain SQL dump (.sql or .sql.gz) into the docker-compose DB.
# Usage: ./scripts/db-restore.sh backups/postgres_spa_booking_20260101_120000.sql.gz
# Run from repo root; requires: docker compose up -d postgres
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <dump.sql|dump.sql.gz>" >&2
  exit 1
fi

DUMP="$1"
if [[ ! -f "$DUMP" ]]; then
  echo "File not found: $DUMP" >&2
  exit 1
fi

if [[ ! -f .env ]]; then
  echo "Missing .env — copy from .env.example and set POSTGRES_PASSWORD." >&2
  exit 1
fi

# shellcheck disable=SC1091
set -a && source .env && set +a

: "${POSTGRES_USER:=spa}"
: "${POSTGRES_DB:=spa_booking}"

echo "Restoring into database $POSTGRES_DB as user $POSTGRES_USER (destructive if dump contains DROP/CLEAN)."
read -r -p "Continue? [y/N] " ok
[[ "${ok:-}" == "y" || "${ok:-}" == "Y" ]] || exit 0

if [[ "$DUMP" == *.gz ]]; then
  gunzip -c "$DUMP" | docker compose exec -T postgres \
    psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"
else
  docker compose exec -T postgres \
    psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" <"$DUMP"
fi

echo "Restore finished."
