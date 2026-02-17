package world

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"mmorp-server/internal/domain/character"
	domainworld "mmorp-server/internal/domain/world"
	"mmorp-server/internal/platform/mq"
)

const (
	playerMoveSpeed        = 0.35
	playerCollisionRadius  = 0.2
	playerAttackRange      = 1.3
	basePlayerDamage       = 20
	mobAggroRange          = 6.0
	mobAttackRange         = 1.1
	mobMoveSpeed           = 0.18
	mobAttackCooldownTicks = 7
	mobRespawnTicks        = 50
	mobWanderMaxTicks      = 20
)

type CharacterPositionUpdater interface {
	UpdatePosition(ctx context.Context, userID, characterID uuid.UUID, x, y float64, zoneID string) error
}

type Client struct {
	Conn        *websocket.Conn
	AccountID   uuid.UUID
	CharacterID uuid.UUID
	Send        chan []byte
}

type MapJSON struct {
	Width  int                    `json:"width"`
	Height int                    `json:"height"`
	Spawn  domainworld.SpawnPoint `json:"spawn"`
	Rows   []string               `json:"rows"`
	NPCs   []domainworld.NPC      `json:"npcs"`
	Mobs   []MobJSON              `json:"mobs"`
}

type MobJSON struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	HP           int     `json:"hp"`
	Damage       int     `json:"damage"`
	PatrolRadius float64 `json:"patrol_radius"`
}

type playerRuntime struct {
	State domainworld.PlayerState
}

type mobRuntime struct {
	State             domainworld.MobState
	SpawnX            float64
	SpawnY            float64
	AttackCooldown    int
	RespawnCounter    int
	WanderDX          float64
	WanderDY          float64
	WanderTicksRemain int
}

type Service struct {
	logger   zerolog.Logger
	pub      mq.Publisher
	updater  CharacterPositionUpdater
	zoneID   string
	tickRate int

	mu       sync.RWMutex
	clients  map[*Client]struct{}
	players  map[uuid.UUID]*playerRuntime
	mobs     map[string]*mobRuntime
	npcs     []domainworld.NPC
	worldMap domainworld.TileMap
	tick     uint64
	quit     chan struct{}
	started  bool
	rand     *rand.Rand
}

type zoneEvent struct {
	ZoneID  string
	Payload any
}

func NewService(logger zerolog.Logger, pub mq.Publisher, updater CharacterPositionUpdater, zoneID string, tickRate int, mapFile string) *Service {
	worldMap, npcs, mobs, err := loadWorldMap(mapFile, zoneID)
	if err != nil {
		logger.Warn().Err(err).Str("map_file", mapFile).Msg("failed to load world map file, using fallback")
		worldMap, npcs, mobs = fallbackWorld(zoneID)
	}
	mobState := make(map[string]*mobRuntime, len(mobs))
	for i := range mobs {
		m := mobs[i]
		mobState[m.ID] = &mobRuntime{
			State:  m,
			SpawnX: m.X,
			SpawnY: m.Y,
		}
	}

	return &Service{
		logger:   logger,
		pub:      pub,
		updater:  updater,
		zoneID:   zoneID,
		tickRate: tickRate,
		clients:  make(map[*Client]struct{}),
		players:  make(map[uuid.UUID]*playerRuntime),
		mobs:     mobState,
		npcs:     npcs,
		worldMap: worldMap,
		quit:     make(chan struct{}),
		rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *Service) Start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	interval := time.Second / time.Duration(s.tickRate)
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.tickWorld()
			case <-s.quit:
				return
			}
		}
	}()
}

func (s *Service) Stop() {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.started = false
	close(s.quit)
	clients := make([]*Client, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.clients = map[*Client]struct{}{}
	s.players = map[uuid.UUID]*playerRuntime{}
	s.mu.Unlock()

	for _, c := range clients {
		close(c.Send)
		if c.Conn != nil {
			_ = c.Conn.Close()
		}
	}
}

func (s *Service) RegisterClient(conn *websocket.Conn, accountID uuid.UUID) *Client {
	c := &Client{Conn: conn, AccountID: accountID, Send: make(chan []byte, 128)}
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()
	return c
}

func (s *Service) UnregisterClient(ctx context.Context, c *Client) {
	s.mu.Lock()
	delete(s.clients, c)
	pr, exists := s.players[c.CharacterID]
	if exists {
		delete(s.players, c.CharacterID)
	}
	s.mu.Unlock()

	if exists {
		s.broadcastZone(c.CharacterID, pr.State.ZoneID, map[string]any{"type": "player_left", "player_id": c.CharacterID})
		s.broadcastZone(uuid.Nil, pr.State.ZoneID, map[string]any{"type": "broadcast", "message": fmt.Sprintf("%s left the world", pr.State.Name)})
		if s.updater != nil {
			if err := s.updater.UpdatePosition(ctx, c.AccountID, c.CharacterID, pr.State.X, pr.State.Y, pr.State.ZoneID); err != nil {
				s.logger.Warn().Err(err).Str("character_id", c.CharacterID.String()).Msg("failed to persist position")
			}
		}
	}
	close(c.Send)
	if c.Conn != nil {
		_ = c.Conn.Close()
	}
}

func (s *Service) Join(c *Client, char character.Character) {
	// Use saved position from DB, fallback to spawn if invalid
	spawnX, spawnY := char.PosX, char.PosY
	if spawnX <= 0 && spawnY <= 0 {
		spawnX, spawnY = s.worldMap.Spawn.X, s.worldMap.Spawn.Y
	}
	// Ensure spawn position is valid
	if !s.isWalkable(spawnX, spawnY) {
		spawnX = 1.5
		spawnY = 1.5
	}

	c.CharacterID = char.ID
	player := domainworld.PlayerState{
		ID:         char.ID,
		Name:       char.Name,
		X:          spawnX,
		Y:          spawnY,
		HP:         100,
		MaxHP:      100,
		Class:      char.Class,
		Level:      1,
		Experience: 0,
		ZoneID:     s.zoneID,
	}

	s.mu.Lock()
	s.players[char.ID] = &playerRuntime{State: player}
	players := s.playersInZoneLocked(s.zoneID)
	mobs := s.mobStatesLocked(s.zoneID)
	npcs := append([]domainworld.NPC(nil), s.npcs...)
	worldMap := s.worldMap
	s.mu.Unlock()

	nonBlockingSendJSON(c.Send, map[string]any{
		"type":       "welcome",
		"selfId":     player.ID,
		"character":  player,
		"zone_id":    s.zoneID,
		"world": map[string]any{
			"zone_id": s.zoneID,
			"map":     worldMap,
			"players": players,
			"mobs":    mobs,
			"npcs":    npcs,
		},
	})

	s.broadcastZone(char.ID, s.zoneID, map[string]any{"type": "player_joined", "player": player})
	s.broadcastZone(uuid.Nil, s.zoneID, map[string]any{"type": "broadcast", "message": fmt.Sprintf("%s joined the world", player.Name)})
}

func (s *Service) Move(c *Client, dx, dy float64) {
	if math.Abs(dx) < 1e-6 && math.Abs(dy) < 1e-6 {
		return
	}
	norm := math.Hypot(dx, dy)
	if norm > 1 {
		dx /= norm
		dy /= norm
	}
	stepX := dx * playerMoveSpeed
	stepY := dy * playerMoveSpeed

	s.mu.Lock()
	pr, ok := s.players[c.CharacterID]
	if !ok {
		s.mu.Unlock()
		return
	}

	nextX := pr.State.X + stepX
	nextY := pr.State.Y
	if s.isWalkableWithRadius(nextX, nextY, playerCollisionRadius) {
		pr.State.X = nextX
	}
	nextX = pr.State.X
	nextY = pr.State.Y + stepY
	if s.isWalkableWithRadius(nextX, nextY, playerCollisionRadius) {
		pr.State.Y = nextY
	}
	newX, newY := pr.State.X, pr.State.Y
	zoneID := pr.State.ZoneID
	s.mu.Unlock()

	s.broadcastZone(uuid.Nil, zoneID, map[string]any{
		"type":      "player_moved",
		"player_id": c.CharacterID,
		"x":         newX,
		"y":         newY,
	})

	// Persist position to DB (async, don't block)
	if s.updater != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := s.updater.UpdatePosition(ctx, c.AccountID, c.CharacterID, newX, newY, zoneID); err != nil {
				s.logger.Warn().Err(err).Str("character_id", c.CharacterID.String()).Msg("position save failed")
			}
		}()
	}
}

func (s *Service) Attack(c *Client, targetID string) {
	s.mu.Lock()
	pr, ok := s.players[c.CharacterID]
	if !ok {
		s.mu.Unlock()
		return
	}
	mob, ok := s.mobs[targetID]
	if !ok || !mob.State.Alive {
		s.mu.Unlock()
		nonBlockingSendJSON(c.Send, map[string]any{"type": "error", "message": "invalid mob target"})
		return
	}

	d := distance(pr.State.X, pr.State.Y, mob.State.X, mob.State.Y)
	if d > playerAttackRange {
		s.mu.Unlock()
		nonBlockingSendJSON(c.Send, map[string]any{"type": "error", "message": "target out of range"})
		return
	}

	dmg := basePlayerDamage + (pr.State.Level-1)*3
	mob.State.HP -= dmg
	zoneID := pr.State.ZoneID
	s.mu.Unlock()

	s.broadcastZone(uuid.Nil, zoneID, map[string]any{"type": "combat", "attacker": c.CharacterID.String(), "target": targetID, "damage": dmg})

	s.mu.Lock()
	mob, ok = s.mobs[targetID]
	if ok && mob.State.HP <= 0 && mob.State.Alive {
		mob.State.Alive = false
		mob.RespawnCounter = mobRespawnTicks
		mob.State.HP = 0
		pr.State.Experience += 25
		for pr.State.Experience >= pr.State.Level*100 {
			pr.State.Experience -= pr.State.Level * 100
			pr.State.Level++
			pr.State.MaxHP += 20
			pr.State.HP = pr.State.MaxHP
		}
	}
	dead := ok && !mob.State.Alive && mob.RespawnCounter == mobRespawnTicks
	playerSnapshot := pr.State
	s.mu.Unlock()

	if dead {
		s.broadcastZone(uuid.Nil, zoneID, map[string]any{"type": "mob_died", "mob_id": targetID})
		s.broadcastZone(uuid.Nil, zoneID, map[string]any{"type": "broadcast", "message": fmt.Sprintf("%s defeated %s", playerSnapshot.Name, targetID)})
		nonBlockingSendJSON(c.Send, map[string]any{"type": "player_update", "player": playerSnapshot})
	}
}

func (s *Service) tickWorld() {
	s.mu.Lock()
	s.tick++
	events := s.stepMobsLocked()
	mobs := s.mobStatesLocked(s.zoneID)
	s.mu.Unlock()

	for _, evt := range events {
		s.broadcastZone(uuid.Nil, evt.ZoneID, evt.Payload)
	}
	s.broadcastZone(uuid.Nil, s.zoneID, map[string]any{"type": "mob_update", "mobs": mobs})
}

func (s *Service) stepMobsLocked() []zoneEvent {
	events := make([]zoneEvent, 0)
	for _, mob := range s.mobs {
		if !mob.State.Alive {
			if mob.RespawnCounter > 0 {
				mob.RespawnCounter--
			}
			if mob.RespawnCounter == 0 {
				mob.State.Alive = true
				mob.State.HP = mob.State.MaxHP
				mob.State.X = mob.SpawnX
				mob.State.Y = mob.SpawnY
				events = append(events, zoneEvent{
					ZoneID: mob.State.ZoneID,
					Payload: map[string]any{
						"type":    "broadcast",
						"message": fmt.Sprintf("%s has respawned", mob.State.Name),
					},
				})
			}
			continue
		}

		target := s.closestPlayerInRangeLocked(mob.State.ZoneID, mob.State.X, mob.State.Y, mobAggroRange)
		if target != nil {
			d := distance(target.State.X, target.State.Y, mob.State.X, mob.State.Y)
			if d <= mobAttackRange {
				if mob.AttackCooldown > 0 {
					mob.AttackCooldown--
				} else {
					events = append(events, s.applyMobAttackLocked(mob, target)...)
					mob.AttackCooldown = mobAttackCooldownTicks
				}
			} else {
				s.moveMobTowardsLocked(mob, target.State.X, target.State.Y)
				if mob.AttackCooldown > 0 {
					mob.AttackCooldown--
				}
			}
			continue
		}

		if mob.AttackCooldown > 0 {
			mob.AttackCooldown--
		}
		s.wanderMobLocked(mob)
	}
	return events
}

func (s *Service) moveMobTowardsLocked(mob *mobRuntime, x, y float64) {
	dx := x - mob.State.X
	dy := y - mob.State.Y
	n := math.Hypot(dx, dy)
	if n < 1e-6 {
		return
	}
	dx = dx / n * mobMoveSpeed
	dy = dy / n * mobMoveSpeed
	nx := mob.State.X + dx
	ny := mob.State.Y + dy
	if s.withinPatrol(mob, nx, ny) && s.isWalkableWithRadius(nx, ny, 0.2) {
		mob.State.X = nx
		mob.State.Y = ny
	}
}

func (s *Service) wanderMobLocked(mob *mobRuntime) {
	if mob.WanderTicksRemain <= 0 {
		ang := s.rand.Float64() * 2 * math.Pi
		mob.WanderDX = math.Cos(ang) * mobMoveSpeed * 0.7
		mob.WanderDY = math.Sin(ang) * mobMoveSpeed * 0.7
		mob.WanderTicksRemain = 5 + s.rand.Intn(mobWanderMaxTicks)
	}
	mob.WanderTicksRemain--
	nx := mob.State.X + mob.WanderDX
	ny := mob.State.Y + mob.WanderDY
	if !s.withinPatrol(mob, nx, ny) || !s.isWalkableWithRadius(nx, ny, 0.2) {
		mob.WanderTicksRemain = 0
		return
	}
	mob.State.X = nx
	mob.State.Y = ny
}

func (s *Service) applyMobAttackLocked(mob *mobRuntime, pr *playerRuntime) []zoneEvent {
	events := []zoneEvent{{
		ZoneID: pr.State.ZoneID,
		Payload: map[string]any{
			"type":     "combat",
			"attacker": mob.State.ID,
			"target":   pr.State.ID.String(),
			"damage":   mob.State.Damage,
		},
	}}
	pr.State.HP -= mob.State.Damage
	if pr.State.HP > 0 {
		return events
	}
	pr.State.HP = pr.State.MaxHP
	pr.State.X = s.worldMap.Spawn.X
	pr.State.Y = s.worldMap.Spawn.Y
	events = append(events, zoneEvent{ZoneID: pr.State.ZoneID, Payload: map[string]any{"type": "player_died", "player_id": pr.State.ID}})
	events = append(events, zoneEvent{ZoneID: pr.State.ZoneID, Payload: map[string]any{"type": "player_moved", "player_id": pr.State.ID, "x": pr.State.X, "y": pr.State.Y}})
	return events
}

func (s *Service) withinPatrol(mob *mobRuntime, x, y float64) bool {
	return distance(mob.SpawnX, mob.SpawnY, x, y) <= mob.State.PatrolRadius
}

func (s *Service) closestPlayerInRangeLocked(zoneID string, x, y, rng float64) *playerRuntime {
	var best *playerRuntime
	bestDist := math.MaxFloat64
	for _, p := range s.players {
		if p.State.ZoneID != zoneID || p.State.HP <= 0 {
			continue
		}
		d := distance(x, y, p.State.X, p.State.Y)
		if d <= rng && d < bestDist {
			best = p
			bestDist = d
		}
	}
	return best
}

func (s *Service) playersInZoneLocked(zoneID string) []domainworld.PlayerState {
	players := make([]domainworld.PlayerState, 0)
	for _, p := range s.players {
		if p.State.ZoneID == zoneID {
			players = append(players, p.State)
		}
	}
	return players
}

func (s *Service) mobStatesLocked(zoneID string) []domainworld.MobState {
	mobs := make([]domainworld.MobState, 0, len(s.mobs))
	for _, m := range s.mobs {
		if m.State.ZoneID == zoneID {
			mobs = append(mobs, m.State)
		}
	}
	return mobs
}

func (s *Service) isWalkable(x, y float64) bool {
	return s.isWalkableWithRadius(x, y, 0)
}

func (s *Service) isWalkableWithRadius(x, y, radius float64) bool {
	checks := [][2]float64{{x, y}, {x - radius, y}, {x + radius, y}, {x, y - radius}, {x, y + radius}}
	for _, c := range checks {
		t := s.tileAt(c[0], c[1])
		if t == domainworld.TileWall || t == domainworld.TileWater {
			return false
		}
	}
	return true
}

func (s *Service) tileAt(x, y float64) domainworld.TileType {
	if x < 0 || y < 0 {
		return domainworld.TileWall
	}
	tx := int(math.Floor(x))
	ty := int(math.Floor(y))
	if tx < 0 || tx >= s.worldMap.Width || ty < 0 || ty >= s.worldMap.Height {
		return domainworld.TileWall
	}
	return s.worldMap.Tiles[ty][tx]
}

func (s *Service) broadcastZone(skipPlayerID uuid.UUID, zoneID string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error().Err(err).Msg("marshal ws payload failed")
		return
	}

	s.mu.RLock()
	clients := make([]*Client, 0, len(s.clients))
	for c := range s.clients {
		if c.CharacterID == uuid.Nil {
			continue
		}
		if skipPlayerID != uuid.Nil && c.CharacterID == skipPlayerID {
			continue
		}
		pr, ok := s.players[c.CharacterID]
		if !ok || pr.State.ZoneID != zoneID {
			continue
		}
		clients = append(clients, c)
	}
	s.mu.RUnlock()

	for _, c := range clients {
		nonBlockingSend(c.Send, b)
	}
}

func (s *Service) WorldState() domainworld.WorldState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	players := make([]domainworld.PlayerState, 0, len(s.players))
	for _, p := range s.players {
		players = append(players, p.State)
	}
	mobs := make([]domainworld.MobState, 0, len(s.mobs))
	for _, m := range s.mobs {
		mobs = append(mobs, m.State)
	}
	npcs := append([]domainworld.NPC(nil), s.npcs...)

	return domainworld.WorldState{
		Tick:    s.tick,
		ZoneID:  s.zoneID,
		Map:     s.worldMap,
		Players: players,
		NPCs:    npcs,
		Mobs:    mobs,
	}
}

func (s *Service) OnlinePlayers() []domainworld.PlayerState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	players := make([]domainworld.PlayerState, 0, len(s.players))
	for _, p := range s.players {
		players = append(players, p.State)
	}
	return players
}

func loadWorldMap(path string, zoneID string) (domainworld.TileMap, []domainworld.NPC, []domainworld.MobState, error) {
	if path == "" {
		return domainworld.TileMap{}, nil, nil, fmt.Errorf("empty world map path")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return domainworld.TileMap{}, nil, nil, fmt.Errorf("read world map: %w", err)
	}
	var data MapJSON
	if err := json.Unmarshal(b, &data); err != nil {
		return domainworld.TileMap{}, nil, nil, fmt.Errorf("parse world map json: %w", err)
	}
	if data.Width <= 0 || data.Height <= 0 {
		return domainworld.TileMap{}, nil, nil, fmt.Errorf("invalid map dimensions")
	}
	if len(data.Rows) != data.Height {
		return domainworld.TileMap{}, nil, nil, fmt.Errorf("rows count must equal height")
	}
	tiles := make([][]domainworld.TileType, data.Height)
	for y := 0; y < data.Height; y++ {
		if len(data.Rows[y]) != data.Width {
			return domainworld.TileMap{}, nil, nil, fmt.Errorf("row %d width mismatch", y)
		}
		row := make([]domainworld.TileType, data.Width)
		for x, r := range data.Rows[y] {
			switch r {
			case '.':
				row[x] = domainworld.TileGrass
			case '~':
				row[x] = domainworld.TileWater
			case '#':
				row[x] = domainworld.TileWall
			case '^':
				row[x] = domainworld.TileForest
			default:
				return domainworld.TileMap{}, nil, nil, fmt.Errorf("unknown tile rune %q", string(r))
			}
		}
		tiles[y] = row
	}

	npcs := make([]domainworld.NPC, 0, len(data.NPCs))
	for _, npc := range data.NPCs {
		npc.ZoneID = zoneID
		npcs = append(npcs, npc)
	}

	mobs := make([]domainworld.MobState, 0, len(data.Mobs))
	for _, m := range data.Mobs {
		if m.ID == "" {
			continue
		}
		hp := m.HP
		if hp <= 0 {
			hp = 60
		}
		dmg := m.Damage
		if dmg <= 0 {
			dmg = 8
		}
		patrol := m.PatrolRadius
		if patrol <= 0 {
			patrol = 5
		}
		mobs = append(mobs, domainworld.MobState{
			ID:           m.ID,
			Name:         m.Name,
			X:            m.X,
			Y:            m.Y,
			HP:           hp,
			MaxHP:        hp,
			Damage:       dmg,
			PatrolRadius: patrol,
			ZoneID:       zoneID,
			Alive:        true,
		})
	}

	return domainworld.TileMap{Width: data.Width, Height: data.Height, Spawn: data.Spawn, Tiles: tiles}, npcs, mobs, nil
}

func fallbackWorld(zoneID string) (domainworld.TileMap, []domainworld.NPC, []domainworld.MobState) {
	width, height := 50, 50
	tiles := make([][]domainworld.TileType, height)
	for y := 0; y < height; y++ {
		row := make([]domainworld.TileType, width)
		for x := 0; x < width; x++ {
			if x == 0 || y == 0 || x == width-1 || y == height-1 {
				row[x] = domainworld.TileWall
				continue
			}
			row[x] = domainworld.TileGrass
		}
		tiles[y] = row
	}
	return domainworld.TileMap{Width: width, Height: height, Spawn: domainworld.SpawnPoint{X: 2.5, Y: 2.5}, Tiles: tiles}, []domainworld.NPC{{ID: "npc-merchant-1", Name: "Rurik", Role: "merchant", X: 5, Y: 5, ZoneID: zoneID}}, []domainworld.MobState{{ID: "mob-slime-1", Name: "Green Slime", X: 14, Y: 12, HP: 60, MaxHP: 60, Damage: 8, PatrolRadius: 6, ZoneID: zoneID, Alive: true}}
}

func distance(ax, ay, bx, by float64) float64 {
	return math.Hypot(ax-bx, ay-by)
}

func nonBlockingSend(ch chan []byte, msg []byte) {
	select {
	case ch <- msg:
	default:
	}
}

func nonBlockingSendJSON(ch chan []byte, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	nonBlockingSend(ch, b)
}
