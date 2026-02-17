package world

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"mmorp-server/internal/domain/character"
)

func TestJoinMoveAndCollision(t *testing.T) {
	svc := NewService(zerolog.Nop(), nil, nil, "starter-zone", 10, "../../../data/maps/starter-zone.json")
	client := svc.RegisterClient(nil, uuid.New())
	charID := uuid.New()
	svc.Join(client, character.Character{ID: charID, Name: "Aria", Class: "mage", ZoneID: "starter-zone"})

	welcome := <-client.Send
	var payload map[string]any
	if err := json.Unmarshal(welcome, &payload); err != nil {
		t.Fatalf("unmarshal welcome: %v", err)
	}
	if payload["type"] != "welcome" {
		t.Fatalf("expected welcome, got %v", payload["type"])
	}

	before := svc.WorldState().Players[0]
	svc.Move(client, -1, 0)
	after := svc.WorldState().Players[0]
	if after.X >= before.X {
		t.Fatalf("expected x to reduce after move; before=%v after=%v", before.X, after.X)
	}

	for i := 0; i < 20; i++ {
		svc.Move(client, -1, 0)
	}
	afterWall := svc.WorldState().Players[0]
	if afterWall.X < 1 {
		t.Fatalf("expected wall collision near x>=1, got x=%v", afterWall.X)
	}
}

func TestAttackAndMobRespawn(t *testing.T) {
	svc := NewService(zerolog.Nop(), nil, nil, "starter-zone", 10, "../../../data/maps/starter-zone.json")
	client := svc.RegisterClient(nil, uuid.New())
	charID := uuid.New()
	svc.Join(client, character.Character{ID: charID, Name: "Aria", Class: "warrior", ZoneID: "starter-zone"})
	<-client.Send

	// Move close to map mob at (16,16).
	for i := 0; i < 40; i++ {
		svc.Move(client, 1, 0)
	}
	for i := 0; i < 40; i++ {
		svc.Move(client, 0, 1)
	}

	svc.Attack(client, "mob-slime-1")
	svc.Attack(client, "mob-slime-1")
	svc.Attack(client, "mob-slime-1")

	state := svc.WorldState()
	var killed bool
	for _, m := range state.Mobs {
		if m.ID == "mob-slime-1" {
			killed = !m.Alive
		}
	}
	if !killed {
		t.Fatalf("expected mob-slime-1 to be dead after attacks")
	}

	for i := 0; i < mobRespawnTicks; i++ {
		svc.tickWorld()
	}

	state = svc.WorldState()
	for _, m := range state.Mobs {
		if m.ID == "mob-slime-1" && !m.Alive {
			t.Fatalf("expected mob-slime-1 to respawn")
		}
	}
}
