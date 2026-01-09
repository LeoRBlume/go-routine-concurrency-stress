# ---- Build stage ----
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Install CA certs for HTTPS module downloads (common on alpine)
RUN apk add --no-cache ca-certificates

# Copy module files first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source
COPY . .

# Build the server entrypoint (cmd/server)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /

COPY --from=builder /out/app /app

EXPOSE 8080

ENTRYPOINT ["/app"]
