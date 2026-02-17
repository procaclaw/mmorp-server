# API Documentation

Base URL (default local): `http://localhost:8080`

## Conventions

- Content type: `application/json`
- Auth header: `Authorization: Bearer <jwt>`
- Protected endpoints require a valid JWT.

## Health

### `GET /healthz`

Response `200`:

```json
{"status":"ok"}
```

### `GET /readyz`

Response `200`:

```json
{"status":"ready"}
```

## Auth

### `POST /v1/auth/register`

Request:

```json
{
  "email": "player@example.com",
  "password": "supersecurepass"
}
```

Success `201`:

```json
{
  "user_id": "8dcf6220-289a-49da-b381-5f4d05d78566",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

Error `400` (invalid body/validation):

```json
{"error":"invalid request"}
```

Error `409` (email already exists):

```json
{"error":"email already in use"}
```

### `POST /v1/auth/login`

Request:

```json
{
  "email": "player@example.com",
  "password": "supersecurepass"
}
```

Success `200`:

```json
{
  "user_id": "8dcf6220-289a-49da-b381-5f4d05d78566",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

Error `401`:

```json
{"error":"invalid credentials"}
```

## Characters (Protected)

### `GET /v1/characters`

Headers:

```http
Authorization: Bearer <jwt>
```

Success `200`:

```json
{
  "items": [
    {
      "id": "29211331-cadf-4e0f-a7cc-7150d5f9f13f",
      "user_id": "8dcf6220-289a-49da-b381-5f4d05d78566",
      "name": "Aria",
      "class": "mage",
      "zone_id": "starter-zone",
      "pos_x": 0,
      "pos_y": 0,
      "created_at": "2026-02-16T19:10:00Z"
    }
  ]
}
```

Error `401`:

```json
{"error":"missing bearer token"}
```

### `POST /v1/characters`

Headers:

```http
Authorization: Bearer <jwt>
```

Request:

```json
{
  "name": "Aria",
  "class": "mage"
}
```

`class` is optional; defaults to `adventurer`.

Success `201`:

```json
{
  "id": "29211331-cadf-4e0f-a7cc-7150d5f9f13f",
  "user_id": "8dcf6220-289a-49da-b381-5f4d05d78566",
  "name": "Aria",
  "class": "mage",
  "zone_id": "starter-zone",
  "pos_x": 0,
  "pos_y": 0,
  "created_at": "2026-02-16T19:10:00Z"
}
```

Error `400`:

```json
{"error":"name required"}
```

### `GET /v1/characters/{characterID}`

Headers:

```http
Authorization: Bearer <jwt>
```

Path params:

- `characterID` (UUID)

Success `200`:

```json
{
  "id": "29211331-cadf-4e0f-a7cc-7150d5f9f13f",
  "user_id": "8dcf6220-289a-49da-b381-5f4d05d78566",
  "name": "Aria",
  "class": "mage",
  "zone_id": "starter-zone",
  "pos_x": 0,
  "pos_y": 0,
  "created_at": "2026-02-16T19:10:00Z"
}
```

Error `400`:

```json
{"error":"invalid character id"}
```

Error `403`:

```json
{"error":"forbidden"}
```

Error `404`:

```json
{"error":"character not found"}
```

## World WebSocket

### `GET /v1/world/ws`

Auth options:

- Query param: `?token=<jwt>`
- Or header: `Authorization: Bearer <jwt>`

On success, endpoint upgrades to WebSocket.

HTTP error `401` (before upgrade):

```json
{"error":"missing token"}
```

or

```json
{"error":"invalid token"}
```

### Client-to-Server Messages

Join world with selected character:

```json
{
  "type": "join",
  "character_id": "29211331-cadf-4e0f-a7cc-7150d5f9f13f"
}
```

Move character:

```json
{
  "type": "move",
  "dx": 1.0,
  "dy": -0.25
}
```

### Server-to-Client Messages

Welcome:

```json
{
  "type": "welcome",
  "character_id": "29211331-cadf-4e0f-a7cc-7150d5f9f13f",
  "zone_id": "starter-zone"
}
```

Snapshot (sent at `WORLD_TICK_RATE`):

```json
{
  "type": "snapshot",
  "tick": 1012,
  "players": [
    {
      "character_id": "29211331-cadf-4e0f-a7cc-7150d5f9f13f",
      "name": "Aria",
      "x": 12.5,
      "y": -3.0,
      "zone_id": "starter-zone"
    }
  ]
}
```

## cURL Examples

Register:

```bash
curl -s http://localhost:8080/v1/auth/register \
  -H 'content-type: application/json' \
  -d '{"email":"player@example.com","password":"supersecurepass"}'
```

Login:

```bash
curl -s http://localhost:8080/v1/auth/login \
  -H 'content-type: application/json' \
  -d '{"email":"player@example.com","password":"supersecurepass"}'
```

List characters:

```bash
curl -s http://localhost:8080/v1/characters \
  -H "authorization: Bearer $TOKEN"
```
