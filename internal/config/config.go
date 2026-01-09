package config

import (
	"os"
	"strconv"
)

// Config holds all runtime configuration for the application.
type Config struct {
	Port              string
	OtelEndpoint      string
	ServiceName       string
	AsyncTimeoutMs    int
	BConcurrencyLimit int
	DisableTraces     bool
}

// Load reads environment variables and returns a populated Config with defaults.
func Load() Config {
	return Config{
		Port:              getEnv("PORT", "8080"),
		OtelEndpoint:      getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel-collector:4318"),
		ServiceName:       getEnv("OTEL_SERVICE_NAME", "go-goroutine-lab"),
		AsyncTimeoutMs:    getEnvInt("ASYNC_TIMEOUT_MS", 600),
		BConcurrencyLimit: getEnvInt("B_CONCURRENCY_LIMIT", 20),
		DisableTraces:     getEnv("OTEL_TRACES_EXPORTER", "") == "none",
	}
}

func getEnv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func getEnvInt(key string, def int) int {
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
