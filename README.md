# Smart Service Booking Platform (Spa chain)

Monorepo for a spa-chain booking product: **Go API** + **PostgreSQL** + **Redis** (Docker Compose), with **`apps/`** reserved for **Next.js** and **React Native** clients.

| Area              | README                                             |
| ----------------- | -------------------------------------------------- |
| **API (Go)**      | [`services/api/README.md`](services/api/README.md) |
| **Frontend apps** | [`apps/README.md`](apps/README.md)                 |

---

## Prerequisites

- **Docker** + **Docker Compose** (recommended for Postgres, Redis, API)
- **Go 1.23+** (optional: run API on host for debugging)
- **Node.js** (when frontend apps are added under `apps/`)

---

## Quick start

1. **Environment**

   ```bash
   cp .env.example .env
   # Set POSTGRES_PASSWORD (required for Docker Compose; e.g. openssl rand -base64 24).
   # Set JWT_SECRET and other secrets; never commit `.env`.
   ```

2. **Start the stack** (from the repository root)

   ```bash
   docker compose up -d --build
   ```

3. **Open**
   - API: `http://localhost:${API_PORT:-8080}`
   - Swagger UI: `http://localhost:${API_PORT:-8080}/swagger/index.html`
   - Postgres (host): `localhost:${POSTGRES_PORT:-5433}`
   - Redis (host): `localhost:${REDIS_PORT:-6379}`

Stop containers (keeps volumes):

```bash
docker compose down
```

**Postgres migrations & backup:** [`scripts/README.md`](scripts/README.md) — one-shot migrate (`docker compose --profile tools run --rm migrate`), `./scripts/db-backup.sh`, `./scripts/db-restore.sh`.

**If a database password was ever pushed to GitHub:** treat it as compromised—rotate `POSTGRES_PASSWORD` (and any DB user password in use), and consider rewriting history ([`git filter-repo`](https://github.com/newren/git-filter-repo)) plus `git push --force` so scanners stop matching old commits. A new commit alone does not remove secrets from past revisions.

---

## Environment variables (repository root)

Defined in [`.env.example`](.env.example). Copy to **`.env`** (gitignored). Docker Compose reads this file for variable substitution.

| Variable                                              | Used by                                         | Description                                                                              |
| ----------------------------------------------------- | ----------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `APP_ENV`                                             | Compose → API container                         | e.g. `development`                                                                       |
| `API_PORT`                                            | Compose                                         | Host port mapped to API **8080** inside container                                        |
| `HTTP_ADDR`                                           | Local `go run`                                  | Listen address (e.g. `:8080`; use `:8081` if host port 8080 is taken)                    |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | Compose → Postgres; builds local `DATABASE_URL` | DB credentials and name                                                                  |
| `POSTGRES_PORT`                                       | Compose                                         | Host port for Postgres (**5433** default avoids local 5432 clashes)                      |
| `REDIS_PORT`                                          | Compose                                         | Host port for Redis                                                                      |
| `DATABASE_URL`                                        | Local Go on host                                | Postgres DSN; may use `${POSTGRES_USER}` etc. — expanded by the API after loading `.env` |
| `REDIS_URL`                                           | Local Go on host                                | Redis URL; may use `${REDIS_PORT}`                                                       |
| `JWT_SECRET`                                          | Compose → API                                   | JWT signing (access + OAuth `state`)                                                     |
| `JWT_ACCESS_TTL`, `JWT_REFRESH_TTL`, …                | Compose → API                                   | Auth TTLs (see `.env.example`)                                                             |
| `GOOGLE_OAUTH_*`, `OAUTH_REDIRECT_URL`                | Compose → API                                   | Google OAuth (optional)                                                                  |

**Inside Docker**, the API service still uses the `DATABASE_URL` / `REDIS_URL` defined in [`docker-compose.yml`](docker-compose.yml) (service hostnames `postgres`, `redis`), not the host-oriented URLs from `.env`.

**Child `.env` examples**

- Frontend (planned): [`apps/.env.example`](apps/.env.example)
- API-only overrides (optional): [`services/api/.env.example`](services/api/.env.example)

---

## Run Go API on the host

Requires Postgres and Redis reachable at the URLs in `.env` (e.g. run the full Compose stack, or only `postgres` + `redis` if you stop the `api` service).

The API loads **`.env`** from the repo root and optionally **`services/api/.env`** (see [`services/api/README.md`](services/api/README.md)); you can run without shell `source`:

```bash
cd services/api && go run ./cmd/server
```

Regenerate Swagger docs after changing `// @...` comments or handler types:

```bash
cd services/api && go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs --parseInternal
```

---

## API overview

REST + Redis pub/sub + WebSocket. Full route list, Swagger regen, and module details: **[`services/api/README.md`](services/api/README.md)**.

| Method | Path                                | Description                              |
| ------ | ----------------------------------- | ---------------------------------------- |
| GET    | `/health`                           | Health check                             |
| GET    | `/api/v1/branches`                  | List branches                            |
| GET    | `/api/v1/services?branch_id=<uuid>` | List services (optional branch filter)   |
| POST   | `/api/v1/bookings`                  | Create booking                           |
| GET    | `/ws/live`                          | WebSocket — Redis channel `spa:bookings` |
| GET    | `/swagger/*`                        | Swagger UI + `doc.json`                  |

Auth (JWT + `principal` consumer/staff, refresh, Google OAuth for consumers, TOTP 2FA; staff roles **admin** / **manager** / **employee**): [`services/api/README.md`](services/api/README.md) — `/api/v1/auth/...`.

Example booking:

```bash
curl -s -X POST "http://localhost:8080/api/v1/bookings" \
  -H "Content-Type: application/json" \
  -d '{
    "branch_id": "BRANCH_UUID",
    "service_id": "SERVICE_UUID",
    "customer_email": "guest@example.com",
    "customer_name": "Nguyen Van A",
    "starts_at": "2026-03-28T09:00:00Z"
  }'
```

---

## Realtime architecture

Each API instance subscribes to Redis `spa:bookings`. New bookings are `PUBLISH`ed; subscribers push events to WebSocket clients on that instance, which supports horizontal scaling of API replicas.

---

## Repository layout

```text
spa-booking-platform/
├── .env.example              # Template for root .env (Compose + local Go)
├── docker-compose.yml
├── skills-lock.json          # Pinned Cursor agent skills (optional)
├── .agents/skills/           # Installed agent skills (Cursor)
├── services/api/             # Go API — see services/api/README.md
│   ├── .env.example          # Optional API-only overrides
│   ├── docs/                 # Generated Swagger (swag)
│   ├── cmd/server/
│   ├── internal/
│   └── Dockerfile
└── apps/                     # Next / RN — see apps/README.md
    └── .env.example          # NEXT_PUBLIC_* for future apps
```

---

## Agent skills (Cursor)

Skills under [`.agents/skills/`](.agents/skills/). Restore from lockfile:

```bash
npx --yes skills experimental_install
```

Install packs:

```bash
npx --yes skills add vercel-labs/agent-skills --skill '*' --agent cursor -y
npx --yes skills add jeffallan/claude-skills@golang-pro --agent cursor -y
```

See skill folders for `SKILL.md` and security notes.

---

## Suggested next steps

- Scaffold **`apps/user-web`** (Next.js) and set `NEXT_PUBLIC_API_URL` — [`apps/README.md`](apps/README.md)
- Add **provider** / **admin** web apps; wire **JWT** using `JWT_SECRET` from `.env`
- **React Native** app under `apps/user-mobile`; use LAN IP for API/WebSocket on devices
