package routers

import (
	"github.com/gin-gonic/gin"

	"go-routine-stress/internal/handlers"
	"go-routine-stress/internal/middleware"
	"go-routine-stress/internal/observability"
)

// NewRouter registers all endpoints and applies per-endpoint instrumentation.
func NewRouter(m *observability.Metrics, h *handlers.Handlers) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", h.Health)

	r.GET("/sync", middleware.Instrument(m, "sync", h.Sync))
	r.GET("/async", middleware.Instrument(m, "async", h.Async))
	r.GET("/async-limited", middleware.Instrument(m, "async-limited", h.AsyncLimited))
	r.GET("/async-timeout", middleware.Instrument(m, "async-timeout", h.AsyncTimeout))

	return r
}
