# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o app main.go

# Final image
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/app ./app
COPY --from=builder /app/config ./config
COPY --from=builder /app/handlers ./handlers
COPY --from=builder /app/service ./service
COPY --from=builder /app/clients ./clients
COPY --from=builder /app/middleware ./middleware
EXPOSE 8080
ENTRYPOINT ["./app"] 