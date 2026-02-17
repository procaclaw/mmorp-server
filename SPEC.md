# MMORPG Server Architecture and Implementation Spec

## 1. Goals and Non-Goals

### Goals
- Build a production-ready skeleton for an authoritative MMORPG backend.
- Support account authentication, character management, and realtime multiplayer state sync.
- Use a coherent modern stack with clear scale path from a single deployment to horizontally scaled services.
- Encode operational best practices: health checks, graceful shutdown, observability hooks, containerization, and externalized config.

### Non-Goals (for this skeleton)
- Full combat, inventory, questing, pathfinding, anti-cheat, and matchmaking systems.
- Full sharding/instancing allocator and dynamic load balancer implementation.
- Full CI/CD pipeline and full security hardening (only baseline foundations are included).

## 2. State-of-the-Art Architecture Research Summary

### Realtime Networking
- MMORPGs are typically authoritative-server systems with fixed simulation ticks and client prediction/interpolation.
- WebSocket is practical for cross-platform clients and fast iteration; UDP/QUIC becomes preferable for high-frequency action and custom reliability channels.
- We choose WebSocket in the skeleton for reach and simplicity, with a migration path to QUIC or custom UDP gateway.

### World/Simulation Model
- A deterministic, tick-based world loop improves consistency and debuggability.
- Spatial partitioning and interest management are required at scale; this skeleton uses a single-zone loop with a clear interface to partition into zones/instances.

### Persistence and Data
- Core player/account/character data needs strong consistency and transactional integrity, favoring SQL.
- Postgres is used as primary source of truth.
- Redis is used as cache and ephemeral shared state.
- Event streaming via NATS enables decoupled side effects and future service decomposition.

### Scaling Pattern
- Start as modular monolith to reduce distributed-system complexity and speed feature delivery.
- Split into services when scale/ownership demands it: auth/profile, world simulation, social/chat, economy, gateway.
- Use stateless API pods + external Postgres/Redis/NATS; scale world pods by zone/instance ownership.

## 3. Monolith vs Microservices Tradeoffs

### Modular Monolith (chosen now)
Pros:
- Easier local development and debugging.
- Simpler consistency and transactions.
- Lower operational burden.

Cons:
- Reduced team autonomy at large org scale.
- Larger blast radius if boundaries are weak.

### Microservices (target trajectory)
Pros:
- Independent scaling by workload type.
- Team ownership boundaries.
- Technology specialization per service.

Cons:
- Network failures, eventual consistency complexity.
- Higher observability, deployment, and SRE demands.

Decision:
- Implement a modular monolith with explicit module boundaries and event interfaces so extraction to microservices is incremental, not a rewrite.

## 4. Data Technology Tradeoffs

### SQL (PostgreSQL) - Primary
- Strong consistency and ACID transactions.
- Rich indexing and relational modeling for accounts/characters/guild/economy.

### NoSQL - Optional adjunct
- Useful for high-volume append-only telemetry, chat history, or flexible documents.
- Not selected as primary in this skeleton to avoid consistency pitfalls for player-critical data.

### NewSQL - Future option
- Useful for globally distributed, strongly consistent SQL at massive scale.
- Candidate when multi-region active/active and write locality become top constraints.

Decision:
- Postgres now, with interfaces that can later support NewSQL migration patterns.

## 5. High-Level System Design

### Runtime Components
- API/Realtime Server (this repo):
  - REST API: auth + character management.
  - WebSocket endpoint for realtime world events.
  - Tick-based world simulation loop.
- PostgreSQL:
  - Source of truth for accounts and characters.
- Redis:
  - Character list caching.
- NATS:
  - Domain event bus for async fan-out.

### Control/Data Flow
1. User registers/logs in (`/v1/auth/*`) and receives JWT.
2. User manages characters (`/v1/characters*`).
3. User joins world websocket with JWT.
4. Client sends movement input; server applies authoritative updates at fixed ticks.
5. Server emits world snapshots to connected players.
6. Server writes persistent state and publishes events.

## 6. Project Structure

- `cmd/server/main.go`: composition root and process lifecycle.
- `internal/platform/*`: infrastructure adapters (config, db, cache, mq, http, migrations).
- `internal/app/*`: use-case/application services.
- `internal/domain/*`: domain models and core rules.
- `internal/api/http_handlers.go`: HTTP and WebSocket handlers.
- `migrations/*.sql`: schema migrations.
- `k8s/*`: deployment manifests.
- `docker-compose.yml`: local stack.

## 7. API Contract (Skeleton)

### Auth
- `POST /v1/auth/register`
  - body: `email`, `password`
  - result: account id + access token
- `POST /v1/auth/login`
  - body: `email`, `password`
  - result: account id + access token

### Characters
- `GET /v1/characters`
  - auth required
- `POST /v1/characters`
  - auth required, body: `name`, optional `class`
- `GET /v1/characters/{id}`
  - auth required, ownership enforced

### Realtime World
- `GET /v1/world/ws?token=<jwt>`
  - websocket upgrade
  - client messages:
    - `join`: `{ "type": "join", "character_id": "..." }`
    - `move`: `{ "type": "move", "dx": <float>, "dy": <float> }`
  - server messages:
    - `welcome`
    - `snapshot` (authoritative positions)

### Health
- `GET /healthz`
- `GET /readyz`

## 8. Security Baseline

- Argon2id password hashing.
- JWT HS256 with configurable secret and expiry.
- Auth middleware for protected endpoints.
- Input validation and request body size limits.
- CORS policy configurable.

## 9. Caching Strategy

- Read-through style for `GET /v1/characters` per user key.
- TTL-based invalidation (short TTL) + write-side delete on mutations.
- Cache-aside behavior on Redis outages (fallback to DB).

## 10. Eventing Strategy

- Publish domain events (e.g., `character.created`, `player.moved`) to NATS.
- Non-blocking publish path (errors logged, request not failed for non-critical events).
- Provides seam for analytics, audit, and async workers.

## 11. Horizontal Scaling Approach

### API Layer
- Stateless; scale behind L4/L7 load balancer.
- JWT enables stateless auth.

### World Layer
- Partition by zone/instance ownership.
- Sticky routing or session directory for websocket affinity.
- Future: dedicated world nodes with distributed coordinator.

### Data Layer
- Postgres with read replicas and partitioning where needed.
- Redis cluster/sentinel for HA.
- NATS cluster for HA messaging.

## 12. Containerization and Operations

- Multi-stage Docker build for minimal runtime image.
- `docker-compose` for local boot of app + Postgres + Redis + NATS.
- Kubernetes manifests for deployment/service/config/secret templates.
- Graceful shutdown handling for HTTP server and world loop.

## 13. Modern Tech Stack Chosen

- Language: Go 1.23+
- HTTP Router: Chi
- Realtime: Gorilla WebSocket
- DB Driver: pgx/v5
- Cache: go-redis/v9
- MQ: nats.go
- Logging: Zerolog
- Auth: JWT + Argon2id
- Container: Docker + Compose
- Orchestration target: Kubernetes

## 14. Implementation Deliverables

- Complete runnable server skeleton with modules above.
- SQL migrations for accounts/characters.
- Production-minded configuration and operational endpoints.
- Basic unit tests for critical logic (auth hashing/token + world update flow).

## 15. Validation Checklist

- `go test ./...` passes.
- `go build ./...` passes.
- App starts with local infra via compose.
- Register/login/character endpoints function.
- WebSocket join/move/snapshot path functions.

## 16. Research Sources

- QUIC transport RFC 9000: https://www.rfc-editor.org/rfc/rfc9000
- WebSocket protocol RFC 6455: https://www.rfc-editor.org/rfc/rfc6455
- PostgreSQL docs (current): https://www.postgresql.org/docs/current/index.html
- Redis docs: https://redis.io/docs/latest/
- NATS docs: https://docs.nats.io/
- Kubernetes docs: https://kubernetes.io/docs/home/
- OpenTelemetry docs: https://opentelemetry.io/docs/
- OWASP Password Storage Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html
- CockroachDB architecture (NewSQL reference): https://www.cockroachlabs.com/docs/stable/architecture/overview
