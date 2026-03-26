#!/usr/bin/env bash
# Rebuild and restart only the API service (faster than rebuilding the whole stack).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
docker compose build api
docker compose up -d api
