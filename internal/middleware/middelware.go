package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"go-routine-stress/internal/observability"
)

// Instrument wraps a handler with basic observability:
// - in-flight tracking
// - request counter
// - latency histogram
func Instrument(m *observability.Metrics, endpoint string, next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Track in-flight requests per endpoint.
		m.IncInflight(endpoint)
		defer m.DecInflight(endpoint)

		// Optional span (no-op if traces are disabled).
		tr := otel.Tracer("go-goroutine-lab/http")
		ctx, span := tr.Start(ctx, "HTTP "+endpoint)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)

		start := time.Now()
		next(c)
		elapsedMs := float64(time.Since(start).Milliseconds())

		status := strconv.Itoa(c.Writer.Status())

		// Attach endpoint and status labels to metrics.
		attrs := metric.WithAttributes(
			attribute.String("endpoint", endpoint),
			attribute.String("status", status),
		)

		m.HTTPRequestsTotal.Add(ctx, 1, attrs)
		m.HTTPRequestDuration.Record(ctx, elapsedMs, attrs)
	}
}
