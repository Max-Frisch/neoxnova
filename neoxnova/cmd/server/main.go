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

	rdb, err := cache.NewRedis(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Fatalf("[FATAL] Failed to connect to Redis: %v", err)
	}
	defer rdb.Close()

	eventEngine := engine.NewEventEngine(rdb, db)
	go eventEngine.StartEventLoop(ctx, cfg.UniverseID)

	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: api.NewRouter(db, rdb, cfg.UniverseID),
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
