package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"mmorp-server/internal/api"
	authapp "mmorp-server/internal/app/auth"
	charapp "mmorp-server/internal/app/character"
	worldapp "mmorp-server/internal/app/world"
	"mmorp-server/internal/platform/cache"
	"mmorp-server/internal/platform/config"
	"mmorp-server/internal/platform/db"
	"mmorp-server/internal/platform/migrate"
	"mmorp-server/internal/platform/mq"
	"mmorp-server/internal/platform/observability"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := observability.NewLogger(cfg.Env)

	pg, err := db.Connect(ctx, cfg.PostgresURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres connection failed")
	}
	defer pg.Close()

	if err := migrate.Up(ctx, pg, cfg.MigrationDir); err != nil {
		logger.Fatal().Err(err).Msg("migrations failed")
	}

	var redisClient *redis.Client
	redisClient, err = cache.New(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Warn().Err(err).Msg("redis unavailable; continuing without cache")
		redisClient = nil
	}
	if redisClient != nil {
		defer redisClient.Close()
	}

	publisher, err := mq.NewPublisher(cfg.NATSURL)
	if err != nil {
		logger.Warn().Err(err).Msg("nats unavailable; using noop publisher")
		publisher = mq.NewNoopPublisher()
	}
	defer publisher.Close()

	authSvc := authapp.NewService(pg, cfg.JWTSecret, cfg.JWTTTL)
	charSvc := charapp.NewService(pg, redisClient, cfg.CharacterTTL, publisher, cfg.WorldZoneID)
	worldSvc := worldapp.NewService(logger, publisher, charSvc, cfg.WorldZoneID, cfg.WorldTickRate, cfg.WorldMapFile)
	worldSvc.Start()
	defer worldSvc.Stop()

	handler := api.NewHandler(logger, authSvc, charSvc, worldSvc, cfg.CorsOrigin, cfg.MaxRequestBody)
	httpServer := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      handler.Router(),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info().Str("addr", cfg.HTTPAddr).Msg("server listening")
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("http server failed")
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
	logger.Info().Msg("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("http shutdown failed")
	}
	logger.Info().Msg("server stopped")
}
