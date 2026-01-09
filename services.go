package main

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type Services struct {
	// Contenção artificial (opcional). Comente o lock/unlock em ServiceB se quiser baseline mais saudável.
	mu sync.Mutex
}

type ServiceAData struct {
	Value   string `json:"value"`
	SleepMs int    `json:"sleepMs"`
}

type ServiceBData struct {
	Value   string `json:"value"`
	SleepMs int    `json:"sleepMs"`
}

func randRange(min, max int) int {
	return min + rand.Intn(max-min+1)
}

// ServiceA: rápido e estável (50–150ms)
// Simula: cache, operação local, baixa variância.
func (s *Services) ServiceA(ctx context.Context) (ServiceAData, error) {
	tr := otel.Tracer("go-goroutine-lab/services")
	_, span := tr.Start(ctx, "ServiceA")
	defer span.End()

	ms := randRange(50, 150)
	span.SetAttributes(
		attribute.Int("sleep_ms", ms),
		attribute.String("profile", "fast-stable"),
	)

	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return ServiceAData{Value: "data-from-A", SleepMs: ms}, nil
	case <-ctx.Done():
		span.SetAttributes(attribute.Bool("canceled", true))
		return ServiceAData{}, ctx.Err()
	}
}

// ServiceB: lento, instável e com erro intermitente (300–1200ms + 5% erro)
// Simula: dependência externa/DB/API.
func (s *Services) ServiceB(ctx context.Context) (ServiceBData, error) {
	tr := otel.Tracer("go-goroutine-lab/services")
	_, span := tr.Start(ctx, "ServiceB")
	defer span.End()

	// 5% de erro simula instabilidade real
	if rand.Float64() < 0.05 {
		span.SetAttributes(attribute.Bool("simulated_error", true))
		return ServiceBData{}, errors.New("service B simulated failure")
	}

	ms := randRange(300, 1200)
	span.SetAttributes(
		attribute.Int("sleep_ms", ms),
		attribute.String("profile", "slow-unstable"),
	)

	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return ServiceBData{Value: "data-from-B", SleepMs: ms}, nil
	case <-ctx.Done():
		span.SetAttributes(attribute.Bool("canceled", true))
		return ServiceBData{}, ctx.Err()
	}
}
