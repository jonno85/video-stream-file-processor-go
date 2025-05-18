# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o app ./cmd/video-stream-file-processor/main.go

# Final image
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/app ./app
COPY --from=builder /app/internal/config ./internal/config
COPY --from=builder /app/internal/handlers ./internal/handlers
COPY --from=builder /app/internal/service ./internal/service
COPY --from=builder /app/internal/adapter ./internal/adapter
COPY --from=builder /app/internal/middleware ./internal/middleware
COPY --from=builder /app/metrics ./metrics
EXPOSE 8080
ENTRYPOINT ["./app"] 