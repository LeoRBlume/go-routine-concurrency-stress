package services

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"
)

// Services simulates external dependencies used by the HTTP handlers.
// Service B is intentionally slower and less reliable to create contention scenarios.
type Services struct {
	// Optional artificial contention. If enabled, ServiceB becomes serialized under load.
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

// New creates a new Services instance.
func New() *Services { return &Services{} }

func randRange(min, max int) int {
	return min + rand.Intn(max-min+1)
}

// ServiceA simulates a fast and stable dependency.
func (s *Services) ServiceA(ctx context.Context) (ServiceAData, error) {
	ms := randRange(50, 150)

	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return ServiceAData{Value: "data-from-A", SleepMs: ms}, nil
	case <-ctx.Done():
		return ServiceAData{}, ctx.Err()
	}
}

// ServiceB simulates a slow and unreliable dependency.
// - 300–1200ms latency
// - 5% error rate
// - optional mutex contention (artificial bottleneck)
func (s *Services) ServiceB(ctx context.Context) (ServiceBData, error) {
	if rand.Float64() < 0.05 {
		return ServiceBData{}, errors.New("service B simulated failure")
	}

	ms := randRange(300, 1200)

	select {
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return ServiceBData{Value: "data-from-B", SleepMs: ms}, nil
	case <-ctx.Done():
		return ServiceBData{}, ctx.Err()
	}
}
