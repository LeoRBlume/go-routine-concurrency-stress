# Go Goroutine Concurrency Lab

This project is a **local, offline concurrency laboratory** built in Go to study **goroutines, synchronization strategies, backpressure, and cancellation**, using **real metrics** exported via **OpenTelemetry → Prometheus → Grafana**.

It is designed for **learning, experimentation, and portfolio demonstration**, not as a production-ready service.

---

## Goals of the Project

This lab exists to answer questions that are often ignored in simple goroutine examples:

- When is `async` actually better than `sync`?
- How does unbounded concurrency degrade a system?
- How does backpressure improve stability?
- Why are timeouts essential to avoid resource leaks?
- How do these behaviors look when measured, not guessed?

All answers are demonstrated using **measured metrics**, not assumptions.

---

## Architecture

Client (k6)
→ Go App (Gin + Goroutines)
→ OpenTelemetry SDK
→ OTel Collector
→ Prometheus
→ Grafana

Everything runs locally using Docker Compose.

---

## Endpoints

Each endpoint performs the same logical work:

- Calls **Service A** (fast, stable)
- Calls **Service B** (slow, variable, error-prone)

The difference is **how concurrency is handled**.

### `/sync`

Sequential execution.

Expected behavior:
- Lowest throughput
- Latency = A + B
- Very stable but slow
- Minimal inflight requests

---

### `/async`

Unbounded parallel execution using goroutines.

Expected behavior:
- Lower latency at low load
- Higher throughput
- Inflight grows quickly under load
- Latency tail (p95/p99) degrades

---

### `/async-limited`

Parallel execution with backpressure.

Expected behavior:
- Slightly higher average latency
- Much better p95/p99 stability
- Inflight stabilizes
- Semaphore wait time becomes visible

---

### `/async-timeout`

Parallel execution with enforced deadline.

Expected behavior:
- Some requests timeout by design
- Inflight drops quickly after spikes
- Goroutine count stabilizes

---

## Services

### Service A
- 50–150ms
- Stable

### Service B
- 300–1200ms
- 5% error rate
- Optional contention

---

## Metrics Collected

- http_requests_total
- http_request_duration_ms
- http_inflight
- service_duration_ms
- service_errors_total
- serviceB_semaphore_wait_ms
- runtime goroutines, memory, GC

---

## How to Run

```bash
docker compose down -v
docker compose up --build
```

App: http://localhost:8080  
Grafana: http://localhost:3000 (admin/admin)

---

## Load Test Example (k6)

Linux:
```bash
docker run --rm -i   --add-host=host.docker.internal:host-gateway   -e TARGET_RPS=30   -e REQ_TIMEOUT=5s   grafana/k6 run - < k6-4endpoints-equal-bursts.js
```

Windows/macOS:
```bash
docker run --rm -i   -e TARGET_RPS=30   -e REQ_TIMEOUT=5s   grafana/k6 run - < k6-4endpoints-equal-bursts.js
```

---

## Interpretation

- Rising inflight → saturation
- Rising p95 after inflight → queueing
- Stable inflight → backpressure working
- Fast inflight drop → effective cancellation

Latency shows the effect. Inflight shows the cause.

---

## License

MIT
