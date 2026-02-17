package world

import "github.com/google/uuid"

type TileType string

const (
	TileGrass  TileType = "grass"
	TileWater  TileType = "water"
	TileWall   TileType = "wall"
	TileForest TileType = "forest"
)

type InteractionType string

const (
	InteractionTypeTalk  InteractionType = "talk"
	InteractionTypeTrade InteractionType = "trade"
	InteractionTypeQuest InteractionType = "quest"
	InteractionTypeHeal  InteractionType = "heal"
)

type SpawnPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type TileMap struct {
	Width  int          `json:"width"`
	Height int          `json:"height"`
	Spawn  SpawnPoint   `json:"spawn"`
	Tiles  [][]TileType `json:"tiles"`
}

type PlayerState struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	X          float64   `json:"x"`
	Y          float64   `json:"y"`
	HP         int       `json:"hp"`
	MaxHP      int       `json:"max_hp"`
	Class      string    `json:"class"`
	Level      int       `json:"level"`
	Experience int       `json:"experience"`
	Gold       int       `json:"gold"`
	ZoneID     string    `json:"zone_id"`
}

type NPC struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Role        string            `json:"role"`
	Interactions []InteractionType `json:"interactions"`
	Dialogue    string            `json:"dialogue,omitempty"`
	TradeItems  []string          `json:"trade_items,omitempty"`
	QuestInfo   string            `json:"quest_info,omitempty"`
	X           float64           `json:"x"`
	Y           float64           `json:"y"`
	ZoneID      string            `json:"zone_id"`
}

type MobState struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	HP           int     `json:"hp"`
	MaxHP        int     `json:"max_hp"`
	Damage       int     `json:"damage"`
	PatrolRadius float64 `json:"patrol_radius"`
	ZoneID       string  `json:"zone_id"`
	Alive        bool    `json:"alive"`
}

type WorldState struct {
	Tick    uint64        `json:"tick"`
	ZoneID  string        `json:"zone_id"`
	Map     TileMap       `json:"map"`
	Players []PlayerState `json:"players"`
	NPCs    []NPC         `json:"npcs"`
	Mobs    []MobState    `json:"mobs"`
}
