package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"go-routine-stress/internal/models"
	"go-routine-stress/internal/observability"
	"go-routine-stress/internal/services"
)

// Handlers contains all HTTP handlers and their dependencies.
type Handlers struct {
	Svcs *services.Services
	M    *observability.Metrics

	// Semaphore used to limit Service B concurrency (backpressure).
	SemB chan struct{}

	// Timeout in milliseconds for /async-timeout.
	TimeoutMs int
}

// New creates a new Handlers instance with dependencies injected.
func New(svcs *services.Services, m *observability.Metrics, semB chan struct{}, timeoutMs int) *Handlers {
	return &Handlers{Svcs: svcs, M: m, SemB: semB, TimeoutMs: timeoutMs}
}

// Health is a simple liveness endpoint.
func (h *Handlers) Health(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

// Sync executes Service A and Service B sequentially.
func (h *Handlers) Sync(c *gin.Context) {
	start := time.Now()
	ctx := c.Request.Context()

	a, errA := h.callServiceA(ctx)
	if errA != nil {
		respondErr(c, "sync", start, http.StatusRequestTimeout, errA)
		return
	}

	b, errB := h.callServiceB(ctx)
	if errB != nil {
		respondErr(c, "sync", start, http.StatusServiceUnavailable, errB)
		return
	}

	c.JSON(http.StatusOK, models.CombinedResponse{
		ServiceAData: a,
		ServiceBData: b,
		Mode:         "sync",
		TotalMs:      time.Since(start).Milliseconds(),
	})
}

// Async executes Service A and Service B concurrently with unbounded goroutines.
func (h *Handlers) Async(c *gin.Context) {
	start := time.Now()
	ctx := c.Request.Context()

	type aRes struct {
		d services.ServiceAData
		e error
	}
	type bRes struct {
		d services.ServiceBData
		e error
	}

	aCh := make(chan aRes, 1)
	bCh := make(chan bRes, 1)

	// Fan-out: start both calls in parallel.
	go func() { d, e := h.callServiceA(ctx); aCh <- aRes{d, e} }()
	go func() { d, e := h.callServiceB(ctx); bCh <- bRes{d, e} }()

	var (
		gotA, gotB bool
		a          services.ServiceAData
		b          services.ServiceBData
		eA, eB     error
	)

	// Fan-in: wait for both results or cancel if context expires.
	for !(gotA && gotB) {
		select {
		case ar := <-aCh:
			a, eA, gotA = ar.d, ar.e, true
		case br := <-bCh:
			b, eB, gotB = br.d, br.e, true
		case <-ctx.Done():
			respondErr(c, "async", start, http.StatusRequestTimeout, ctx.Err())
			return
		}
	}

	if eA != nil || eB != nil {
		respondErr(c, "async", start, http.StatusServiceUnavailable, fmt.Errorf("A:%v B:%v", eA, eB))
		return
	}

	c.JSON(http.StatusOK, models.CombinedResponse{
		ServiceAData: a,
		ServiceBData: b,
		Mode:         "async",
		TotalMs:      time.Since(start).Milliseconds(),
	})
}

// AsyncLimited executes concurrently, but applies backpressure to Service B using a semaphore.
func (h *Handlers) AsyncLimited(c *gin.Context) {
	start := time.Now()
	ctx := c.Request.Context()

	type aRes struct {
		d services.ServiceAData
		e error
	}
	type bRes struct {
		d services.ServiceBData
		e error
	}

	aCh := make(chan aRes, 1)
	bCh := make(chan bRes, 1)

	go func() { d, e := h.callServiceA(ctx); aCh <- aRes{d, e} }()

	// Service B is protected by a semaphore (backpressure).
	go func() {
		waitStart := time.Now()

		select {
		case h.SemB <- struct{}{}:
			// Record how long we waited to enter the limited section.
			h.M.SemWaitB.Record(ctx, float64(time.Since(waitStart).Milliseconds()),
				metric.WithAttributes(attribute.String("endpoint", "async-limited")),
			)
			defer func() { <-h.SemB }()
		case <-ctx.Done():
			bCh <- bRes{services.ServiceBData{}, ctx.Err()}
			return
		}

		d, e := h.callServiceB(ctx)
		bCh <- bRes{d, e}
	}()

	var (
		gotA, gotB bool
		a          services.ServiceAData
		b          services.ServiceBData
		eA, eB     error
	)

	for !(gotA && gotB) {
		select {
		case ar := <-aCh:
			a, eA, gotA = ar.d, ar.e, true
		case br := <-bCh:
			b, eB, gotB = br.d, br.e, true
		case <-ctx.Done():
			respondErr(c, "async-limited", start, http.StatusRequestTimeout, ctx.Err())
			return
		}
	}

	if eA != nil || eB != nil {
		respondErr(c, "async-limited", start, http.StatusServiceUnavailable, fmt.Errorf("A:%v B:%v", eA, eB))
		return
	}

	c.JSON(http.StatusOK, models.CombinedResponse{
		ServiceAData: a,
		ServiceBData: b,
		Mode:         "async-limited",
		TotalMs:      time.Since(start).Milliseconds(),
	})
}

// AsyncTimeout enforces a deadline using context cancellation.
func (h *Handlers) AsyncTimeout(c *gin.Context) {
	start := time.Now()
	parent := c.Request.Context()

	ctx, cancel := context.WithTimeout(parent, time.Duration(h.TimeoutMs)*time.Millisecond)
	defer cancel()

	type aRes struct {
		d services.ServiceAData
		e error
	}
	type bRes struct {
		d services.ServiceBData
		e error
	}

	aCh := make(chan aRes, 1)
	bCh := make(chan bRes, 1)

	go func() { d, e := h.callServiceA(ctx); aCh <- aRes{d, e} }()
	go func() { d, e := h.callServiceB(ctx); bCh <- bRes{d, e} }()

	var (
		gotA, gotB bool
		a          services.ServiceAData
		b          services.ServiceBData
		eA, eB     error
	)

	for !(gotA && gotB) {
		select {
		case ar := <-aCh:
			a, eA, gotA = ar.d, ar.e, true
		case br := <-bCh:
			b, eB, gotB = br.d, br.e, true
		case <-ctx.Done():
			respondErr(c, "async-timeout", start, http.StatusRequestTimeout, ctx.Err())
			return
		}
	}

	if eA != nil || eB != nil {
		respondErr(c, "async-timeout", start, http.StatusServiceUnavailable, fmt.Errorf("A:%v B:%v", eA, eB))
		return
	}

	c.JSON(http.StatusOK, models.CombinedResponse{
		ServiceAData: a,
		ServiceBData: b,
		Mode:         "async-timeout",
		TotalMs:      time.Since(start).Milliseconds(),
	})
}

// callServiceA wraps Service A with metrics.
func (h *Handlers) callServiceA(ctx context.Context) (services.ServiceAData, error) {
	start := time.Now()
	d, err := h.Svcs.ServiceA(ctx)

	h.M.ServiceDuration.Record(ctx, float64(time.Since(start).Milliseconds()),
		metric.WithAttributes(attribute.String("service", "A")),
	)
	if err != nil {
		h.M.ServiceErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("service", "A")))
	}
	return d, err
}

// callServiceB wraps Service B with metrics.
func (h *Handlers) callServiceB(ctx context.Context) (services.ServiceBData, error) {
	start := time.Now()
	d, err := h.Svcs.ServiceB(ctx)

	h.M.ServiceDuration.Record(ctx, float64(time.Since(start).Milliseconds()),
		metric.WithAttributes(attribute.String("service", "B")),
	)
	if err != nil {
		h.M.ServiceErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("service", "B")))
	}
	return d, err
}

func respondErr(c *gin.Context, mode string, start time.Time, status int, err error) {
	c.JSON(status, models.ErrorResponse{
		Mode:    mode,
		TotalMs: time.Since(start).Milliseconds(),
		Error:   err.Error(),
	})
}
