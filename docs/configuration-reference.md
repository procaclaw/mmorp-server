# Configuration Reference

Environment variables are loaded in `internal/platform/config/config.go`.

## Server & HTTP

| Variable | Default | Required | Description |
|---|---|---|---|
| `APP_ENV` | `dev` | No | Runtime environment. `dev` enables console-style debug logging; others use JSON/info logging. |
| `HTTP_ADDR` | `:8080` | No | HTTP bind address. |
| `CORS_ORIGIN` | `*` | No | Value set in `Access-Control-Allow-Origin`. |
| `HTTP_READ_TIMEOUT` | `15s` | No | HTTP server read timeout (`time.ParseDuration` format). |
| `HTTP_WRITE_TIMEOUT` | `15s` | No | HTTP server write timeout. |
| `HTTP_SHUTDOWN_TIMEOUT` | `20s` | No | Graceful shutdown timeout. |
| `MAX_REQUEST_BODY_BYTES` | `1048576` | No | Max request body size enforced by JSON decoder wrapper. |

## Auth

| Variable | Default | Required | Description |
|---|---|---|---|
| `JWT_SECRET` | `change-me` | Yes (non-empty) | HMAC secret used for JWT signing/verification. Empty is rejected at startup. |
| `JWT_TTL` | `24h` | No | JWT expiration duration. |

## Database & Migrations

| Variable | Default | Required | Description |
|---|---|---|---|
| `POSTGRES_URL` | `postgres://postgres:postgres@localhost:5432/mmorp?sslmode=disable` | Yes | Postgres connection URL for pgx pool and migrations. |
| `MIGRATION_DIR` | `migrations` | No | Directory containing SQL migration files. |

## Redis Cache

| Variable | Default | Required | Description |
|---|---|---|---|
| `REDIS_ADDR` | `localhost:6379` | No | Redis address. If unreachable, server continues without cache. |
| `REDIS_PASSWORD` | `` | No | Redis password. |
| `REDIS_DB` | `0` | No | Redis DB index. |
| `CHARACTER_CACHE_TTL` | `30s` | No | TTL for per-user character list cache entries. |

## Messaging / World

| Variable | Default | Required | Description |
|---|---|---|---|
| `NATS_URL` | `nats://localhost:4222` | No | NATS server URL. If unreachable, server uses noop publisher. |
| `WORLD_TICK_RATE` | `20` | Yes (`>0`) | Snapshot tick frequency per second. Startup fails if not positive. |
| `WORLD_ZONE_ID` | `starter-zone` | No | Default zone for new characters and movement events. |

## Example

See `.env.example` for a complete local configuration template.
