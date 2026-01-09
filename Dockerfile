FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app .

FROM alpine:3.20
WORKDIR /
COPY --from=builder /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
