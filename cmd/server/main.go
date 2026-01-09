package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	"go-routine-stress/internal/config"
	"go-routine-stress/internal/handlers"
	"go-routine-stress/internal/observability"
	"go-routine-stress/internal/routers"
	"go-routine-stress/internal/services"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	cfg := config.Load()

	// Initialize OpenTelemetry (metrics + optional traces).
	shutdown, err := observability.SetupOTel(context.Background(), cfg.OtelEndpoint, cfg.ServiceName, cfg.DisableTraces)
	if err != nil {
		log.Fatalf("otel init failed: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	m, err := observability.NewMetrics()
	if err != nil {
		log.Fatalf("metrics init failed: %v", err)
	}

	// Create simulated dependencies (Service A and Service B).
	svcs := services.New()

	// Semaphore used to apply backpressure on Service B (async-limited endpoint).
	semB := make(chan struct{}, cfg.BConcurrencyLimit)

	h := handlers.New(svcs, m, semB, cfg.AsyncTimeoutMs)

	r := routers.NewRouter(m, h)

	log.Printf("listening on :%s", cfg.Port)
	log.Fatal(r.Run(":" + cfg.Port))
}
