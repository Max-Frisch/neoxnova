package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"neoxnova/internal/api"
	"neoxnova/internal/cache"
	"neoxnova/internal/config"
	"neoxnova/internal/engine"
	"neoxnova/internal/store"
)

func main() {
	log.Println("[INIT] Launching Neo-XNova Game Engine v2.0...")

	// Load .env for local development. Absence is not fatal.
	if err := godotenv.Load(); err != nil {
		log.Printf("[INIT] No .env file loaded, using process environment: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Failed to load configuration: %v", err)
	}

	db, err := store.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[FATAL] Failed to connect to PostgreSQL: %v", err)
	}
	defer db.Close()

	// Apply the config/game.json tuning onto the universes row on every boot;
	// the DB then stays the single runtime source for speeds and queue slots.
	gameSettings, err := config.LoadGameSettings(cfg.GameConfigPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to load game config: %v", err)
	}
	if applied, err := store.ApplyGameSettings(ctx, db, cfg.UniverseID, gameSettings); err != nil {
		log.Fatalf("[FATAL] Failed to apply game settings to universe %q: %v", cfg.UniverseID, err)
	} else if applied {
		log.Printf("[INIT] Applied game tuning: game_speed=%g resource_speed=%g fleet_speed=%g build_slots=%d research_slots=%d merchant_rate=%d dm_divisor=%d",
			gameSettings.GameSpeed, gameSettings.ResourceSpeed, gameSettings.FleetSpeed,
			gameSettings.BuildQueueSlots, gameSettings.ResearchQueueSlots,
			gameSettings.MerchantRate, gameSettings.DMMarketDivisor)
	} else {
		log.Printf("[WARN] Universe %q not found; game settings not applied (run `make migrate && make seed`)", cfg.UniverseID)
	}

	// Redis is an optional wake-up accelerator; the durable scheduler polls
	// Postgres regardless, so startup proceeds even if Redis is unavailable.
	rdb, err := cache.NewRedis(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Printf("[WARN] Redis unavailable (%v); continuing without wake accelerator", err)
		rdb = nil
	} else {
		defer rdb.Close()
	}

	eventEngine := engine.NewEventEngine(rdb, db)
	go eventEngine.StartScheduler(ctx, cfg.UniverseID)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewRouter(db, rdb, cfg.UniverseID),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("[INIT] Web API Gateway listening on HTTP %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[FATAL] Web server crashed on startup: %v", err)
		}
	}()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
	<-stopChan

	log.Println("[SHUTDOWN] Stopping Neo-XNova Engine gracefully...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Web server shutdown forced an exception error: %v", err)
	}

	log.Println("[SHUTDOWN] Engine completely closed down cleanly. Goodbye!")
}
