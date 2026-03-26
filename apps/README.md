# Spa Booking — Frontend apps (`apps/`)

Planned **Next.js** and **React Native / Expo** applications. The API lives in [`services/api`](../services/api/README.md); shared infrastructure is documented at the [repository root](../README.md).

## Planned layout

| Directory | Stack | Audience |
| --------- | ----- | -------- |
| `user-web` | Next.js | Customers: browse branches/services, book |
| `user-mobile` | React Native / Expo | Same domain as user web |
| `provider-web` | Next.js | Branch staff: schedule, confirm bookings |
| `admin-web` | Next.js | Chain operations: branches, services, reports |

Scaffold when ready (example):

```bash
cd apps
npx create-next-app@latest user-web --typescript --eslint --app
```

## Environment variables

Copy [`.env.example`](.env.example) per app (Next.js commonly uses **`.env.local`**, gitignored).

| Variable | Description |
| -------- | ----------- |
| `NEXT_PUBLIC_API_URL` | REST base URL (e.g. `http://localhost:8080`) |
| `NEXT_PUBLIC_WS_URL` | WebSocket URL (e.g. `ws://localhost:8080` for `/ws/live`) |

**Local web:** `localhost` is fine. **Physical devices / emulators:** use your machine’s **LAN IP** instead of `localhost` so the client can reach the API.

## Backend stack

From the [repository root](../README.md), start Postgres, Redis, and the API:

```bash
docker compose up -d --build
docker compose down   # stop when done
```

Then run each app’s dev server from its folder (`npm run dev`, `npx expo start`, etc.).

## CORS

The API allows selected local origins (see [`internal/handler/router.go`](../services/api/internal/handler/router.go)). When you add a new web app port or host, update CORS there or move allowed origins to configuration.
