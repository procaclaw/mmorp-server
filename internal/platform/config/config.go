package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env            string
	HTTPAddr       string
	CorsOrigin     string
	JWTSecret      string
	JWTTTL         time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	ShutdownTimout time.Duration

	PostgresURL    string
	MigrationDir   string
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	CharacterTTL   time.Duration
	NATSURL        string
	WorldTickRate  int
	WorldZoneID    string
	WorldMapFile   string
	MaxRequestBody int64
}

func Load() (Config, error) {
	cfg := Config{
		Env:            getEnv("APP_ENV", "dev"),
		HTTPAddr:       getEnv("HTTP_ADDR", "192.168.30.254:8080"),
		CorsOrigin:     getEnv("CORS_ORIGIN", "*"),
		JWTSecret:      getEnv("JWT_SECRET", "change-me"),
		JWTTTL:         getDuration("JWT_TTL", 24*time.Hour),
		ReadTimeout:    getDuration("HTTP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:   getDuration("HTTP_WRITE_TIMEOUT", 15*time.Second),
		ShutdownTimout: getDuration("HTTP_SHUTDOWN_TIMEOUT", 20*time.Second),
		PostgresURL:    getEnv("POSTGRES_URL", "postgres://postgres:postgres@localhost:5432/mmorp?sslmode=disable"),
		MigrationDir:   getEnv("MIGRATION_DIR", "migrations"),
		RedisAddr:      getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:  getEnv("REDIS_PASSWORD", ""),
		RedisDB:        getInt("REDIS_DB", 0),
		CharacterTTL:   getDuration("CHARACTER_CACHE_TTL", 30*time.Second),
		NATSURL:        getEnv("NATS_URL", "nats://localhost:4222"),
		WorldTickRate:  getInt("WORLD_TICK_RATE", 10),
		WorldZoneID:    getEnv("WORLD_ZONE_ID", "starter-zone"),
		WorldMapFile:   getEnv("WORLD_MAP_FILE", "data/maps/starter-zone.json"),
		MaxRequestBody: getInt64("MAX_REQUEST_BODY_BYTES", 1<<20),
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET must not be empty")
	}
	if cfg.WorldTickRate <= 0 {
		return Config{}, fmt.Errorf("WORLD_TICK_RATE must be > 0")
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getInt64(key string, def int64) int64 {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func getDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
