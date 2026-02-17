# Module Design

## Composition Root

### `cmd/server`

Purpose:

- Bootstrap process, wire dependencies, own lifecycle.

Responsibilities:

- Load config.
- Connect Postgres, Redis, NATS.
- Run migrations.
- Construct app services and HTTP handler.
- Start world simulation loop.
- Start/stop HTTP server gracefully.

Public interface:

- `main()`

## API Module

### `internal/api`

Purpose:

- Translate HTTP/WebSocket traffic into application service calls.

Responsibilities:

- Route registration.
- Auth middleware (JWT parsing via auth service).
- Request decoding, body size limits, JSON responses.
- WebSocket read/write pumps and protocol handling.

Public interfaces:

- `NewHandler(logger, authSvc, charSvc, worldSvc, corsOrigin, maxBodySize) *Handler`
- `(*Handler).Router() http.Handler`

## Application Modules

### `internal/app/auth`

Purpose:

- Account registration/login and token operations.

Responsibilities:

- Normalize email.
- Validate credential input.
- Hash/verify password with Argon2id.
- Persist/fetch user credentials.
- Issue and parse JWT tokens.

Public interfaces:

- `NewService(db, jwtSecret, jwtTTL) *Service`
- `(*Service).Register(ctx, email, password) (AuthResult, error)`
- `(*Service).Login(ctx, email, password) (AuthResult, error)`
- `(*Service).ParseToken(tokenString) (uuid.UUID, error)`

### `internal/app/character`

Purpose:

- Character CRUD-style ownership operations and state persistence.

Responsibilities:

- Create character with defaults.
- List user characters (cache-aside through Redis).
- Resolve character by ID with ownership checks.
- Persist position updates from world service.
- Publish character creation events.

Public interfaces:

- `NewService(db, cache, cacheTTL, publisher, zoneID) *Service`
- `(*Service).Create(ctx, userID, name, class) (character.Character, error)`
- `(*Service).ListByUser(ctx, userID) ([]character.Character, error)`
- `(*Service).GetByIDForUser(ctx, userID, characterID) (character.Character, error)`
- `(*Service).UpdatePosition(ctx, userID, characterID, x, y, zoneID) error`

### `internal/app/world`

Purpose:

- In-memory authoritative world state and snapshot broadcast loop.

Responsibilities:

- Track connected clients.
- Track active player states in zone.
- Handle join and move commands.
- Broadcast periodic snapshots.
- Publish movement events.
- Persist final position on client disconnect.

Public interfaces:

- `NewService(logger, publisher, positionUpdater, zoneID, tickRate) *Service`
- `(*Service).Start()`
- `(*Service).Stop()`
- `(*Service).RegisterClient(conn, accountID) *Client`
- `(*Service).UnregisterClient(ctx, client)`
- `(*Service).Join(client, char)`
- `(*Service).Move(client, dx, dy)`

## Domain Modules

### `internal/domain/auth`

- `User` entity model.

### `internal/domain/character`

- `Character` entity model used by API and application layers.

### `internal/domain/world`

- `PlayerState` and `Snapshot` realtime payload models.

## Platform Modules

### `internal/platform/config`

- Loads environment variables into strongly typed `Config`.

### `internal/platform/db`

- Builds and validates PostgreSQL connection pool.

### `internal/platform/cache`

- Creates Redis client and validates connectivity.

### `internal/platform/mq`

- Defines `Publisher` abstraction.
- Provides NATS-backed publisher and noop fallback publisher.

### `internal/platform/migrate`

- Applies ordered `.sql` migrations transactionally.
- Maintains `schema_migrations` table.

### `internal/platform/observability`

- Configures zerolog format and log level by environment.
