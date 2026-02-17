package character

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"mmorp-server/internal/domain/character"
	"mmorp-server/internal/platform/mq"
)

var ErrNotFound = errors.New("character not found")
var ErrForbidden = errors.New("forbidden")

type Service struct {
	db       *pgxpool.Pool
	cache    *redis.Client
	cacheTTL time.Duration
	pub      mq.Publisher
	zoneID   string
}

func NewService(db *pgxpool.Pool, cache *redis.Client, cacheTTL time.Duration, pub mq.Publisher, zoneID string) *Service {
	return &Service{db: db, cache: cache, cacheTTL: cacheTTL, pub: pub, zoneID: zoneID}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, name, class string) (character.Character, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return character.Character{}, fmt.Errorf("name required")
	}
	if class == "" {
		class = "adventurer"
	}
	id := uuid.New()
	var c character.Character
	err := s.db.QueryRow(ctx, `
INSERT INTO characters (id, user_id, name, class, zone_id, pos_x, pos_y)
VALUES ($1, $2, $3, $4, $5, 0, 0)
RETURNING id, user_id, name, class, zone_id, pos_x, pos_y, created_at
`, id, userID, name, class, s.zoneID).Scan(&c.ID, &c.UserID, &c.Name, &c.Class, &c.ZoneID, &c.PosX, &c.PosY, &c.CreatedAt)
	if err != nil {
		return character.Character{}, fmt.Errorf("insert character: %w", err)
	}
	s.invalidateCharacterList(ctx, userID)
	_ = s.publishEvent(ctx, "character.created", map[string]any{"character_id": c.ID, "user_id": c.UserID})
	return c, nil
}

func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID) ([]character.Character, error) {
	key := s.cacheKey(userID)
	if s.cache != nil {
		cached, err := s.cache.Get(ctx, key).Result()
		if err == nil {
			var chars []character.Character
			if uErr := json.Unmarshal([]byte(cached), &chars); uErr == nil {
				return chars, nil
			}
		}
	}

	rows, err := s.db.Query(ctx, `
SELECT id, user_id, name, class, zone_id, pos_x, pos_y, created_at
FROM characters WHERE user_id = $1 ORDER BY created_at ASC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("query characters: %w", err)
	}
	defer rows.Close()

	chars := make([]character.Character, 0)
	for rows.Next() {
		var c character.Character
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Class, &c.ZoneID, &c.PosX, &c.PosY, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan character: %w", err)
		}
		chars = append(chars, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate characters: %w", err)
	}
	if s.cache != nil {
		if b, err := json.Marshal(chars); err == nil {
			_ = s.cache.Set(ctx, key, b, s.cacheTTL).Err()
		}
	}
	return chars, nil
}

func (s *Service) GetByIDForUser(ctx context.Context, userID, characterID uuid.UUID) (character.Character, error) {
	var c character.Character
	err := s.db.QueryRow(ctx, `
SELECT id, user_id, name, class, zone_id, pos_x, pos_y, created_at
FROM characters WHERE id = $1
`, characterID).Scan(&c.ID, &c.UserID, &c.Name, &c.Class, &c.ZoneID, &c.PosX, &c.PosY, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return character.Character{}, ErrNotFound
		}
		return character.Character{}, fmt.Errorf("query character: %w", err)
	}
	if c.UserID != userID {
		return character.Character{}, ErrForbidden
	}
	return c, nil
}

func (s *Service) UpdatePosition(ctx context.Context, userID, characterID uuid.UUID, x, y float64, zoneID string) error {
	res, err := s.db.Exec(ctx, `
UPDATE characters
SET pos_x = $1, pos_y = $2, zone_id = $3, updated_at = NOW()
WHERE id = $4 AND user_id = $5
`, x, y, zoneID, characterID, userID)
	if err != nil {
		return fmt.Errorf("update position: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrForbidden
	}
	s.invalidateCharacterList(ctx, userID)
	return nil
}

func (s *Service) cacheKey(userID uuid.UUID) string {
	return "characters:user:" + userID.String()
}

func (s *Service) invalidateCharacterList(ctx context.Context, userID uuid.UUID) {
	if s.cache == nil {
		return
	}
	_ = s.cache.Del(ctx, s.cacheKey(userID)).Err()
}

func (s *Service) publishEvent(ctx context.Context, subject string, payload any) error {
	if s.pub == nil {
		return nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.pub.Publish(ctx, subject, b)
}
