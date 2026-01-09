package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

/* ============================
   Models
============================ */

type CombinedResponse struct {
	ServiceAData ServiceAData `json:"serviceAData"`
	ServiceBData ServiceBData `json:"serviceBData"`
	Mode         string       `json:"mode"`
	TotalMs      int64        `json:"totalMs"`
}

type ErrorResponse struct {
	Mode    string `json:"mode"`
	TotalMs int64  `json:"totalMs"`
	Error   string `json:"error"`
}

/* ============================
   Metrics
============================ */

var (
	meter metric.Meter

	httpRequestsTotal   metric.Int64Counter
	httpRequestDuration metric.Float64Histogram

	serviceDuration metric.Float64Histogram
	serviceErrors   metric.Int64Counter

	semWaitB metric.Float64Histogram

	// inflight gauge (por endpoint)
	inflightByEndpoint sync.Map // map[string]*atomic.Int64
)

func inflightInc(endpoint string) {
	v, _ := inflightByEndpoint.LoadOrStore(endpoint, &atomic.Int64{})
	v.(*atomic.Int64).Add(1)
}

func inflightDec(endpoint string) {
	if v, ok := inflightByEndpoint.Load(endpoint); ok {
		v.(*atomic.Int64).Add(-1)
	}
}

/* ============================
   Main
============================ */

func main() {
	rand.Seed(time.Now().UnixNano())

	ctx := context.Background()
	shutdown, err := initOTel(ctx)
	if err != nil {
		log.Fatalf("otel init failed: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	initMetrics()

	svcs := &Services{}

	semB := make(chan struct{}, envInt("B_CONCURRENCY_LIMIT", 20))
	timeoutMs := envInt("ASYNC_TIMEOUT_MS", 600)

	r := gin.New()
	r.Use(gin.Recovery())
	// r.Use(gin.Logger()) // habilite se quiser logar requests

	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	/* -------- SYNC -------- */
	r.GET("/sync", instrumentGin("sync", func(c *gin.Context) {
		start := time.Now()
		ctx := c.Request.Context()

		a, errA := callServiceA(ctx, svcs)
		if errA != nil {
			respondErr(c, "sync", start, http.StatusRequestTimeout, errA)
			return
		}

		b, errB := callServiceB(ctx, svcs)
		if errB != nil {
			respondErr(c, "sync", start, http.StatusServiceUnavailable, errB)
			return
		}

		c.JSON(http.StatusOK, CombinedResponse{
			ServiceAData: a,
			ServiceBData: b,
			Mode:         "sync",
			TotalMs:      time.Since(start).Milliseconds(),
		})
	}))

	/* -------- ASYNC -------- */
	r.GET("/liync", instrumentGin("async", func(c *gin.Context) {
		start := time.Now()
		ctx := c.Request.Context()

		type aRes struct {
			data ServiceAData
			err  error
		}
		type bRes struct {
			data ServiceBData
			err  error
		}

		aCh := make(chan aRes, 1)
		bCh := make(chan bRes, 1)

		go func() { d, e := callServiceA(ctx, svcs); aCh <- aRes{d, e} }()
		go func() { d, e := callServiceB(ctx, svcs); bCh <- bRes{d, e} }()

		var (
			gotA bool
			gotB bool
			a    ServiceAData
			b    ServiceBData
			eA   error
			eB   error
		)

		for !(gotA && gotB) {
			select {
			case ar := <-aCh:
				a, eA, gotA = ar.data, ar.err, true
			case br := <-bCh:
				b, eB, gotB = br.data, br.err, true
			case <-ctx.Done():
				respondErr(c, "async", start, http.StatusRequestTimeout, ctx.Err())
				return
			}
		}

		if eA != nil || eB != nil {
			respondErr(c, "async", start, http.StatusServiceUnavailable, fmt.Errorf("a:%v B:%v", eA, eB))
			return
		}

		c.JSON(http.StatusOK, CombinedResponse{
			ServiceAData: a,
			ServiceBData: b,
			Mode:         "async",
			TotalMs:      time.Since(start).Milliseconds(),
		})
	}))

	/* -------- ASYNC-LIMITED -------- */
	r.GET("/async-limited", instrumentGin("async-limited", func(c *gin.Context) {
		start := time.Now()
		ctx := c.Request.Context()

		type aRes struct {
			data ServiceAData
			err  error
		}
		type bRes struct {
			data ServiceBData
			err  error
		}

		aCh := make(chan aRes, 1)
		bCh := make(chan bRes, 1)

		go func() { d, e := callServiceA(ctx, svcs); aCh <- aRes{d, e} }()

		go func() {
			waitStart := time.Now()
			select {
			case semB <- struct{}{}:
				semWaitB.Record(ctx, float64(time.Since(waitStart).Milliseconds()),
					metric.WithAttributes(attribute.String("endpoint", "async-limited")),
				)
				defer func() { <-semB }()
			case <-ctx.Done():
				bCh <- bRes{ServiceBData{}, ctx.Err()}
				return
			}

			d, e := callServiceB(ctx, svcs)
			bCh <- bRes{d, e}
		}()

		var (
			gotA bool
			gotB bool
			a    ServiceAData
			b    ServiceBData
			eA   error
			eB   error
		)

		for !(gotA && gotB) {
			select {
			case ar := <-aCh:
				a, eA, gotA = ar.data, ar.err, true
			case br := <-bCh:
				b, eB, gotB = br.data, br.err, true
			case <-ctx.Done():
				respondErr(c, "async-limited", start, http.StatusRequestTimeout, ctx.Err())
				return
			}
		}

		if eA != nil || eB != nil {
			respondErr(c, "async-limited", start, http.StatusServiceUnavailable, fmt.Errorf("a:%v B:%v", eA, eB))
			return
		}

		c.JSON(http.StatusOK, CombinedResponse{
			ServiceAData: a,
			ServiceBData: b,
			Mode:         "async-limited",
			TotalMs:      time.Since(start).Milliseconds(),
		})
	}))

	/* -------- ASYNC-TIMEOUT -------- */
	r.GET("/async-timeout", instrumentGin("async-timeout", func(c *gin.Context) {
		start := time.Now()
		parent := c.Request.Context()
		ctx, cancel := context.WithTimeout(parent, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()

		type aRes struct {
			data ServiceAData
			err  error
		}
		type bRes struct {
			data ServiceBData
			err  error
		}

		aCh := make(chan aRes, 1)
		bCh := make(chan bRes, 1)

		go func() { d, e := callServiceA(ctx, svcs); aCh <- aRes{d, e} }()
		go func() { d, e := callServiceB(ctx, svcs); bCh <- bRes{d, e} }()

		var (
			gotA bool
			gotB bool
			a    ServiceAData
			b    ServiceBData
			eA   error
			eB   error
		)

		for !(gotA && gotB) {
			select {
			case ar := <-aCh:
				a, eA, gotA = ar.data, ar.err, true
			case br := <-bCh:
				b, eB, gotB = br.data, br.err, true
			case <-ctx.Done():
				respondErr(c, "async-timeout", start, http.StatusRequestTimeout, ctx.Err())
				return
			}
		}

		if eA != nil || eB != nil {
			respondErr(c, "async-timeout", start, http.StatusServiceUnavailable, fmt.Errorf("a:%v B:%v", eA, eB))
			return
		}

		c.JSON(http.StatusOK, CombinedResponse{
			ServiceAData: a,
			ServiceBData: b,
			Mode:         "async-timeout",
			TotalMs:      time.Since(start).Milliseconds(),
		})
	}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on :%s", port)
	log.Fatal(r.Run(":" + port))
}

/* ============================
   Middleware
============================ */

func instrumentGin(endpoint string, h gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		inflightInc(endpoint)
		defer inflightDec(endpoint)

		tr := otel.Tracer("go-goroutine-lab/http")
		ctx, span := tr.Start(ctx, "HTTP "+endpoint)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)

		start := time.Now()
		h(c)
		elapsedMs := float64(time.Since(start).Milliseconds())

		status := strconv.Itoa(c.Writer.Status())

		attrs := metric.WithAttributes(
			attribute.String("endpoint", endpoint),
			attribute.String("status", status),
		)

		httpRequestsTotal.Add(ctx, 1, attrs)
		httpRequestDuration.Record(ctx, elapsedMs, attrs)
	}
}

/* ============================
   Service wrappers
============================ */

func callServiceA(ctx context.Context, svcs *Services) (ServiceAData, error) {
	start := time.Now()
	d, err := svcs.ServiceA(ctx)
	serviceDuration.Record(ctx, float64(time.Since(start).Milliseconds()),
		metric.WithAttributes(attribute.String("service", "A")),
	)
	if err != nil {
		serviceErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("service", "A")))
	}
	return d, err
}

func callServiceB(ctx context.Context, svcs *Services) (ServiceBData, error) {
	start := time.Now()
	d, err := svcs.ServiceB(ctx)
	serviceDuration.Record(ctx, float64(time.Since(start).Milliseconds()),
		metric.WithAttributes(attribute.String("service", "B")),
	)
	if err != nil {
		serviceErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("service", "B")))
	}
	return d, err
}

/* ============================
   Metrics init
============================ */

func initMetrics() {
	meter = otel.Meter("go-goroutine-lab/metrics")

	httpRequestsTotal, _ = meter.Int64Counter("http_requests_total")
	httpRequestDuration, _ = meter.Float64Histogram("http_request_duration_ms")
	serviceDuration, _ = meter.Float64Histogram("service_duration_ms")
	serviceErrors, _ = meter.Int64Counter("service_errors_total")
	semWaitB, _ = meter.Float64Histogram("serviceB_semaphore_wait_ms")

	// http_inflight gauge
	_, _ = meter.Int64ObservableGauge("http_inflight",
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			inflightByEndpoint.Range(func(k, v any) bool {
				obs.Observe(v.(*atomic.Int64).Load(),
					metric.WithAttributes(attribute.String("endpoint", k.(string))),
				)
				return true
			})
			return nil
		}),
	)
}

/* ============================
   OTel init
============================ */

func initOTel(ctx context.Context) (func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://otel-collector:4318"
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName("go-goroutine-lab")),
	)

	metricExp, _ := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint))
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(3*time.Second))),
	)
	otel.SetMeterProvider(mp)

	if os.Getenv("OTEL_TRACES_EXPORTER") != "none" {
		traceExp, _ := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithBatcher(traceExp),
		)
		otel.SetTracerProvider(tp)
	}

	_ = runtime.Start(runtime.WithMinimumReadMemStatsInterval(2 * time.Second))

	return func(ctx context.Context) error {
		_ = mp.Shutdown(ctx)
		return nil
	}, nil
}

/* ============================
   Helpers
============================ */

func respondErr(c *gin.Context, mode string, start time.Time, status int, err error) {
	c.JSON(status, ErrorResponse{
		Mode:    mode,
		TotalMs: time.Since(start).Milliseconds(),
		Error:   err.Error(),
	})
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
