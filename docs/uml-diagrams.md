# UML Diagrams (Mermaid)

## Class Diagram (Domain + Key Services)

```mermaid
classDiagram
    class User {
      +UUID ID
      +string Email
      +string PasswordHash
      +time.Time CreatedAt
    }

    class Character {
      +UUID ID
      +UUID UserID
      +string Name
      +string Class
      +string ZoneID
      +float64 PosX
      +float64 PosY
      +time.Time CreatedAt
    }

    class PlayerState {
      +UUID CharacterID
      +string Name
      +float64 X
      +float64 Y
      +string ZoneID
    }

    class Snapshot {
      +string Type
      +uint64 Tick
      +[]PlayerState Players
    }

    class AuthService {
      +Register(ctx,email,password) AuthResult
      +Login(ctx,email,password) AuthResult
      +ParseToken(token) UUID
    }

    class CharacterService {
      +Create(ctx,userID,name,class) Character
      +ListByUser(ctx,userID) []Character
      +GetByIDForUser(ctx,userID,characterID) Character
      +UpdatePosition(ctx,userID,characterID,x,y,zoneID) error
    }

    class WorldService {
      +Start()
      +Stop()
      +RegisterClient(conn,accountID) Client
      +UnregisterClient(ctx,client)
      +Join(client,char)
      +Move(client,dx,dy)
    }

    User "1" --> "many" Character : owns
    Character --> PlayerState : mapped to
    Snapshot --> PlayerState : contains
    WorldService --> CharacterService : persist position
```

## Sequence Diagram: Register

```mermaid
sequenceDiagram
    participant C as Client
    participant H as HTTP Handler
    participant A as Auth Service
    participant DB as PostgreSQL

    C->>H: POST /v1/auth/register (email,password)
    H->>A: Register(ctx,email,password)
    A->>A: normalize + validate + hash password
    A->>DB: INSERT INTO users(...)
    DB-->>A: inserted user
    A->>A: issue JWT
    A-->>H: {user_id, token}
    H-->>C: 201 Created
```

## Sequence Diagram: Login

```mermaid
sequenceDiagram
    participant C as Client
    participant H as HTTP Handler
    participant A as Auth Service
    participant DB as PostgreSQL

    C->>H: POST /v1/auth/login (email,password)
    H->>A: Login(ctx,email,password)
    A->>DB: SELECT id,password_hash FROM users WHERE email=...
    DB-->>A: user row
    A->>A: verify Argon2id hash
    A->>A: issue JWT
    A-->>H: {user_id, token}
    H-->>C: 200 OK
```

## Sequence Diagram: Create Character

```mermaid
sequenceDiagram
    participant C as Client
    participant H as HTTP Handler
    participant M as Auth Middleware
    participant CS as Character Service
    participant DB as PostgreSQL
    participant R as Redis
    participant N as NATS

    C->>H: POST /v1/characters (Bearer token)
    H->>M: auth check
    M->>H: user_id in context
    H->>CS: Create(ctx,user_id,name,class)
    CS->>DB: INSERT INTO characters(...)
    DB-->>CS: created character row
    CS->>R: DEL characters:user:<user_id>
    CS->>N: publish character.created
    CS-->>H: Character
    H-->>C: 201 Created
```

## Sequence Diagram: World Join and Move

```mermaid
sequenceDiagram
    participant C as Client
    participant H as WS Handler
    participant A as Auth Service
    participant CS as Character Service
    participant W as World Service
    participant DB as PostgreSQL
    participant N as NATS

    C->>H: GET /v1/world/ws?token=jwt (upgrade)
    H->>A: ParseToken(jwt)
    A-->>H: user_id
    H->>W: RegisterClient(conn,user_id)

    C->>H: {"type":"join","character_id":"..."}
    H->>CS: GetByIDForUser(user_id,character_id)
    CS-->>H: Character
    H->>W: Join(client, character)
    W-->>C: {"type":"welcome",...}

    loop movement input
      C->>H: {"type":"move","dx":...,"dy":...}
      H->>W: Move(client,dx,dy)
      W->>N: publish player.moved
    end

    loop world tick
      W-->>C: {"type":"snapshot","tick":...,"players":[...]}
    end

    C--xH: disconnect
    H->>W: UnregisterClient(...)
    W->>CS: UpdatePosition(...)
    CS->>DB: UPDATE characters SET pos_x,pos_y,zone_id...
```

## Component Diagram

```mermaid
flowchart LR
    subgraph ClientSide
      GC[Game Client]
    end

    subgraph Server["mmorp-server"]
      API[HTTP/WS API]
      AUTH[Auth Module]
      CHAR[Character Module]
      WORLD[World Module]
      MIG[Migration Module]
    end

    PG[(PostgreSQL)]
    REDIS[(Redis)]
    NATS[(NATS)]

    GC --> API
    API --> AUTH
    API --> CHAR
    API --> WORLD
    MIG --> PG
    AUTH --> PG
    CHAR --> PG
    CHAR <--> REDIS
    CHAR --> NATS
    WORLD --> NATS
    WORLD --> CHAR
```

## State Diagram: Character Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Created
    Created --> Selected : player joins world
    Selected --> InWorld : join accepted
    InWorld --> Moving : move command
    Moving --> InWorld : tick snapshot
    InWorld --> PersistingPosition : disconnect
    PersistingPosition --> Offline : DB update success
    Offline --> Selected : reconnect + join
    Offline --> Deleted : owner deleted (cascade)
    Deleted --> [*]
```

## State Diagram: World/Connection Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Idle
    Idle --> Connected : websocket upgraded
    Connected --> Authenticated : token parsed
    Authenticated --> Registered : client registered in world service
    Registered --> Joined : valid join message
    Joined --> Simulating : receiving move + tick snapshots
    Simulating --> Disconnecting : socket closed/error
    Disconnecting --> Persisted : position update attempted
    Persisted --> Closed
    Closed --> [*]
```
