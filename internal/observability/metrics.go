package observability

import (
	"context"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics groups all metric instruments in one place.
type Metrics struct {
	HTTPRequestsTotal   metric.Int64Counter
	HTTPRequestDuration metric.Float64Histogram

	ServiceDuration metric.Float64Histogram
	ServiceErrors   metric.Int64Counter

	SemWaitB metric.Float64Histogram

	// Inflight is exported as an observable gauge per endpoint.
	inflight sync.Map // map[string]*atomic.Int64
}

// NewMetrics creates all instruments and registers callbacks.
func NewMetrics() (*Metrics, error) {
	m := &Metrics{}
	meter := otel.Meter("go-goroutine-lab/metrics")

	var err error

	m.HTTPRequestsTotal, err = meter.Int64Counter("http_requests_total")
	if err != nil {
		return nil, err
	}
	m.HTTPRequestDuration, err = meter.Float64Histogram("http_request_duration_ms")
	if err != nil {
		return nil, err
	}

	m.ServiceDuration, err = meter.Float64Histogram("service_duration_ms")
	if err != nil {
		return nil, err
	}
	m.ServiceErrors, err = meter.Int64Counter("service_errors_total")
	if err != nil {
		return nil, err
	}

	m.SemWaitB, err = meter.Float64Histogram("serviceB_semaphore_wait_ms")
	if err != nil {
		return nil, err
	}

	// http_inflight gauge reports current in-flight requests per endpoint.
	_, err = meter.Int64ObservableGauge("http_inflight",
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			m.inflight.Range(func(k, v any) bool {
				endpoint := k.(string)
				val := v.(*atomic.Int64).Load()
				obs.Observe(val, metric.WithAttributes(attribute.String("endpoint", endpoint)))
				return true
			})
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// IncInflight increments the in-flight counter for an endpoint.
func (m *Metrics) IncInflight(endpoint string) {
	v, _ := m.inflight.LoadOrStore(endpoint, &atomic.Int64{})
	v.(*atomic.Int64).Add(1)
}

// DecInflight decrements the in-flight counter for an endpoint.
func (m *Metrics) DecInflight(endpoint string) {
	if v, ok := m.inflight.Load(endpoint); ok {
		v.(*atomic.Int64).Add(-1)
	}
}
