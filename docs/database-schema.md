# Database Schema

Primary database is PostgreSQL. Migrations are in `migrations/`.

## Tables

### `users`

- `id UUID PRIMARY KEY`
- `email TEXT NOT NULL UNIQUE`
- `password_hash TEXT NOT NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

### `characters`

- `id UUID PRIMARY KEY`
- `user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE`
- `name TEXT NOT NULL`
- `class TEXT NOT NULL`
- `zone_id TEXT NOT NULL`
- `pos_x DOUBLE PRECISION NOT NULL DEFAULT 0`
- `pos_y DOUBLE PRECISION NOT NULL DEFAULT 0`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
- `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

Indexes:

- `idx_characters_user_id` on `characters(user_id)`

### `schema_migrations`

- `version TEXT PRIMARY KEY`
- `applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`

Used by the internal migration runner to track applied SQL files.

## Relationship Summary

- One `users` row has many `characters` rows.
- `characters.user_id` enforces ownership and referential integrity.
- Deleting a user cascades delete to owned characters.

## ERD (Mermaid)

```mermaid
erDiagram
    USERS ||--o{ CHARACTERS : owns

    USERS {
        UUID id PK
        TEXT email UNIQUE
        TEXT password_hash
        TIMESTAMPTZ created_at
    }

    CHARACTERS {
        UUID id PK
        UUID user_id FK
        TEXT name
        TEXT class
        TEXT zone_id
        DOUBLE pos_x
        DOUBLE pos_y
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }

    SCHEMA_MIGRATIONS {
        TEXT version PK
        TIMESTAMPTZ applied_at
    }
```
