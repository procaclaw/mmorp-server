package character

import (
	"time"

	"github.com/google/uuid"
)

type Character struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Class     string    `json:"class"`
	ZoneID    string    `json:"zone_id"`
	PosX      float64   `json:"pos_x"`
	PosY      float64   `json:"pos_y"`
	CreatedAt time.Time `json:"created_at"`
}
