# mmorp-server

Production-ready MMORPG backend in Go.

## Features

### World & Map
- **Tile-based map system** (50x50 default): grass (`.`), water (`~`), wall (`#`), forest (`^`)
- **Collision detection**: players cannot walk through walls or water
- **Custom maps**: load any JSON map file with the `WORLD_MAP_FILE` env var

### NPCs
- Static entities that stay in place
- Two types in starter zone: merchant (Rurik), quest_giver (Elda)
- Displayed on client with name labels

### Mobs (Enemies)
- AI-controlled enemies with patrol behavior
- Chase players within detection range
- Attack and deal damage
- Drop XP on death (respawn after 30s)

**Starter zone mobs:**
- Green Slime (HP: 60, Damage: 8)
- Blue Slime (HP: 70, Damage: 9)
- Forest Wolf (HP: 95, Damage: 12)

### Combat
- Attack with `{"type":"attack","targetId":"<mob-id>"}`
- Damage calculation with slight randomness
- Floating combat text on client
- Mob death → XP reward → respawn timer

### World Simulation
- Server-authoritative movement (client predictions verified)
- 10 ticks/second tick rate (configurable)
- Mob AI runs each tick: wander, chase, attack
- WebSocket broadcasts: player_joined, player_left, player_moved, mob_update, combat, player_died

## Quickstart

1. Start dependencies and server:

```bash
docker compose up --build
```

2. Register:

```bash
curl -s http://localhost:8080/v1/auth/register \
  -H 'content-type: application/json' \
  -d '{"email":"player@example.com","password":"supersecurepass"}'
```

3. Login:

```bash
curl -s http://localhost:8080/v1/auth/login \
  -H 'content-type: application/json' \
  -d '{"email":"player@example.com","password":"supersecurepass"}'
```

4. Create character:

```bash
curl -s http://localhost:8080/v1/characters \
  -H "authorization: Bearer $TOKEN" \
  -H 'content-type: application/json' \
  -d '{"name":"Aria","class":"mage"}'
```

5. Connect WebSocket:

`ws://localhost:8080/v1/world/ws?token=$TOKEN`

Message examples:

```json
{"type":"join","character_id":"<character-uuid>"}
{"type":"move","dx":1,"dy":0}
{"type":"attack","targetId":"mob-slime-1"}
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `WORLD_ZONE_ID` | `starter-zone` | Zone/area identifier |
| `WORLD_MAP_FILE` | `data/maps/starter-zone.json` | Path to map JSON file |
| `WORLD_TICK_RATE` | `10` | Ticks per second (mob AI speed) |
| `JWT_SECRET` | - | Secret for JWT signing |
| `POSTGRES_URL` | - | PostgreSQL connection string |
| `REDIS_ADDR` | `redis:6379` | Redis address |
| `NATS_URL` | `nats://nats:4222` | NATS server URL |

## Custom Maps

Create a JSON map file:

```json
{
  "width": 30,
  "height": 30,
  "spawn": {"x": 2.5, "y": 2.5},
  "rows": [
    "##############################",
    "#............................#",
    "#...~~~~~~~~~~~~~~~~~~~~~~~~~#",
    "#...~~~~~~~~~~~~~~~~~~~~~~~~~#",
    "#...~~~~~~~~~~~~~~~~~~~~~~~~~#",
    "#............................#",
    "##############################"
  ],
  "npcs": [
    {"id": "my-npc", "name": "Guard", "role": "merchant", "x": 5, "y": 5}
  ],
  "mobs": [
    {"id": "goblin", "name": "Goblin", "x": 10, "y": 10, "hp": 50, "damage": 5, "patrol_radius": 4}
  ]
}
```

Tile legend: `.` = grass, `~` = water, `#` = wall, `^` = forest

Run with custom map:
```bash
docker run -p 8080:8080 -e WORLD_MAP_FILE=/app/data/maps/my-map.json -v /path/to/maps:/app/data/maps mmorp-server
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/auth/register` | POST | Create account |
| `/v1/auth/login` | POST | Get JWT token |
| `/v1/characters` | POST | Create character |
| `/v1/characters` | GET | List your characters |
| `/v1/characters/:id` | DELETE | Delete character |
| `/v1/world/state` | GET | Debug: full world state |
| `/v1/world/players` | GET | Debug: online players |
| `/health` | GET | Health check |
| `/ready` | GET | Readiness check |

## WebSocket Protocol

### Client → Server
```json
{"type":"join","character_id":"uuid"}
{"type":"move","dx":1,"dy":0}
{"type":"attack","targetId":"mob-slime-1"}
```

### Server → Client
```json
{"type":"welcome","player":{...},"world":{...}}
{"type":"player_joined","player":{...}}
{"type":"player_left","player_id":"uuid"}
{"type":"player_moved","player_id":"uuid","x":5,"y":6}
{"type":"mob_update","mobs":[...]}
{"type":"combat","target_id":"mob-slime-1","damage":15,"hp":45}
{"type":"player_died","message":"You died!"}
{"type":"error","message":"..."}
```
