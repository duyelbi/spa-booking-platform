#!/usr/bin/env bash
# Interactive psql in the postgres container. Extra args are passed to psql.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
  echo "Missing .env — copy from .env.example." >&2
  exit 1
fi
# shellcheck disable=SC1091
set -a && source .env && set +a
: "${POSTGRES_USER:=spa}"
: "${POSTGRES_DB:=spa_booking}"

docker compose exec postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" "$@"
