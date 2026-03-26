# Spa Booking — API (`services/api`)

Go **1.23** HTTP service: **Chi**, **pgx**, **Redis**, **WebSocket** hub, embedded SQL migrations, **Swagger** (swag).

**Parent docs:** [Repository root `README.md`](../../README.md) (monorepo layout, Docker, shared `.env`).

## Prerequisites

- **Docker stack:** PostgreSQL + Redis (via root `docker compose`) _or_ your own instances on the same ports as `DATABASE_URL` / `REDIS_URL`.
- **Local Go:** Go 1.23+ when running `go run` / tests on the host.

## Environment variables

At startup the server calls **`config.LoadEnvFiles()`** ([`internal/config/dotenv.go`](internal/config/dotenv.go)) using [godotenv](https://github.com/joho/godotenv), then reads **`os.Getenv`**. Values may contain **`${VAR}`** / **`$VAR`**; those are expanded with [`os.ExpandEnv`](https://pkg.go.dev/os#ExpandEnv) after load (so `DATABASE_URL` can reference `POSTGRES_USER`, etc.).

**Files loaded (only if they exist), merge order — later overrides earlier:**

| Current working directory | Files                            |
| ------------------------- | -------------------------------- |
| Repository root           | `.env`, then `services/api/.env` |
| `services/api`            | `../../.env`, then `.env`        |

**Precedence:** variables already set in the real OS environment (Docker, shell `export`, CI) are **not** replaced by `.env` (godotenv default).

| Variable       | Purpose                   | Default (if unset)                                                     |
| -------------- | ------------------------- | ---------------------------------------------------------------------- |
| `HTTP_ADDR`    | Listen address            | `:8080`                                                                |
| `DATABASE_URL` | Postgres DSN              | Set in `.env` (see root `.env.example`); no default password in repo |
| `REDIS_URL`    | Redis URL                 | `redis://localhost:6379/0`                                             |
| `APP_ENV`      | Environment label         | `development`                                                          |
| `JWT_SECRET`   | HS256 access tokens + OAuth state | `dev-secret-change-me` |
| `JWT_ACCESS_TTL`, `JWT_REFRESH_TTL`, … | Auth token lifetimes | see root [`.env.example`](../../.env.example) |
| `GOOGLE_OAUTH_*`, `OAUTH_REDIRECT_URL` | Google OAuth (optional) | empty disables |

With Docker Compose from the repo root, the **container** gets `DATABASE_URL` / `REDIS_URL` pointing at service names (`postgres`, `redis`). On the **host**, point at published ports (see root [`.env.example`](../../.env.example): typically `127.0.0.1:5433` for Postgres).

Optional overrides for this folder only: [`.env.example`](.env.example).

**Migrations:** applied on every API startup (`internal/db.Migrate`). To run SQL only: `go run ./cmd/migrate` from this directory, or from repo root `docker compose --profile tools run --rm migrate`. **Backup / restore:** see [`scripts/README.md`](../../scripts/README.md).

## Authentication (`/api/v1/auth`)

Identity lives in **one Postgres database** under schemas `consumer` (guests) and `staff` (admin / manager / employee) — no legacy `public.users` table:

| Schema | Tables | Purpose |
| ------ | ------ | ------- |
| `consumer` | `accounts`, `oauth_accounts`, `refresh_tokens` | Khách đặt lịch / đăng ký app |
| `staff` | `accounts`, `profiles`, `oauth_accounts`, `refresh_tokens` | Admin, **manager**, **employee** (nhân viên) |

- **Register** and **Google OAuth** only create **consumer** rows. Staff accounts are created with SQL (or a future admin API).
- **`POST /login`** tries **staff** first (same email), then **consumer**.
- Access JWTs include `role` and **`principal`**: `consumer` (role `user`) or `staff` (role `admin` \| `manager` \| `employee`). Refresh tokens are stored in the matching schema.

| Method | Path | Auth | Description |
| ------ | ---- | ---- | ----------- |
| POST | `/register` | — | Consumer sign-up: email + password → access + refresh |
| POST | `/login` | — | Sign in (staff checked first, then consumer). 2FA → `temp_token` + `two_factor_required` |
| POST | `/login/2fa` | — | `temp_token` + TOTP code → tokens |
| POST | `/refresh` | — | Rotate refresh, new access |
| POST | `/logout` | — | Revoke refresh |
| GET | `/oauth/google/url` | — | `authorization_url` |
| GET | `/oauth/google/callback` | — | Google redirect → JSON tokens (consumer only) |
| GET | `/me` | Bearer | Profile (`principal` + `role`) |
| POST | `/2fa/setup` \| `/2fa/enable` \| `/2fa/disable` | Bearer | TOTP |
| GET | `/staff/ping` | Bearer | **employee**, **manager**, or **admin** |
| GET | `/admin/ping` | Bearer | **admin** only |

Create a staff login (example: admin). Set `password_hash` to a bcrypt hash from your app or a one-off script:

```sql
INSERT INTO staff.accounts (email, full_name, password_hash, email_verified)
VALUES ('admin@spa.local', 'Admin', '<bcrypt_hash>', true)
RETURNING id;

INSERT INTO staff.profiles (account_id, role) VALUES ('<id_from_above>', 'admin');
-- roles: admin | manager | employee
```

`bookings.customer_id` references `consumer.accounts(id)`; `bookings.staff_id` references `staff.accounts(id)` (nullable).

## Docker Compose (repository root)

```bash
# from repository root
docker compose up -d --build    # Postgres + Redis + API
docker compose down             # stop (volumes kept)
```

## Run on the host (`go run`)

With a root `.env` (and optional `services/api/.env`), you do **not** need `source .env` in the shell. Run from this module directory (`go.mod` is here):

```bash
cd services/api && go run ./cmd/server
```

You can still `export` variables manually; they win over `.env` files.

If the Docker API already binds **8080**, set `HTTP_ADDR=:8081` in `.env` before `go run`.

## Build & test

```bash
cd services/api
go build -o server ./cmd/server
go test ./...
```

## Docker image

Built from this directory (see [`Dockerfile`](Dockerfile)). Compose builds `api` with context `.` here.

```bash
# from repository root
docker compose build api
```

## Swagger / OpenAPI

- **UI:** `http://localhost:<API_PORT>/swagger/index.html` (default port **8080**).
- **JSON:** `/swagger/doc.json`

Regenerate after changing `// @...` comments or exported handler types:

```bash
cd services/api
go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/server/main.go -o docs --parseInternal
```

Do not edit `docs/docs.go`, `swagger.json`, or `swagger.yaml` by hand.

## HTTP routes (summary)

| Method | Path                          | Notes                           |
| ------ | ----------------------------- | ------------------------------- |
| GET    | `/health`                     | Liveness                        |
| GET    | `/api/v1/branches`            | List branches                   |
| GET    | `/api/v1/services?branch_id=` | List services (optional filter) |
| POST   | `/api/v1/bookings`            | Create booking                  |
| GET    | `/ws/live`                    | WebSocket (not in OpenAPI)      |
| GET    | `/swagger/*`                  | Swagger UI + spec               |

## Migrations

SQL files live in [`internal/db/migrations/`](internal/db/migrations/): **`001_init`** (branches, catalog, `consumer.*`, `staff.*`, `bookings`) and **`002_seed`**. Applied in lexical order on every startup (`//go:embed`). **Greenfield only:** if you had an older schema with `public.users`, reset the DB volume (`docker compose down -v`) or create a new database.

## Module path

```text
spa-booking/services/api
```
