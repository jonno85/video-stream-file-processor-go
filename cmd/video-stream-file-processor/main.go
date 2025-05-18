// Package main implements the entry point for the video stream file processor service.
// It sets up background workers, HTTP server, and integrates with S3, Redis, and Prometheus for metrics.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jonno85/video-stream-file-processor/internal/adapter"
	"github.com/jonno85/video-stream-file-processor/internal/config"
	"github.com/jonno85/video-stream-file-processor/internal/handlers"
	"github.com/jonno85/video-stream-file-processor/internal/middleware"
	"github.com/jonno85/video-stream-file-processor/internal/service"
	"github.com/minio/minio-go/v7"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ensureS3Bucket ensures the S3 bucket exists, creating it if it does not. Exits the program on error.
func ensureS3Bucket(s3Client *adapter.S3ClientImpl, s3Bucket string) {
	exists, err := s3Client.BucketExists(context.Background(), s3Bucket)
	if err != nil {
		slog.Error("Failed to check if bucket exists", "err", err)
		os.Exit(1)
	}
	if !exists {
		err := s3Client.MakeBucket(context.Background(), s3Bucket, minio.MakeBucketOptions{
			Region: "eu-west-1",
		})
		if err != nil {
			slog.Error("Failed to create bucket", "err", err)
			os.Exit(1)
		}
		slog.Info("Bucket created", "bucket", s3Bucket)
	}
}

// runBackgroundTasks starts background goroutines for processing pending queues, watching paths, and processing the main queue.
func runBackgroundTasks(videoFileProcessorService *service.VideoFileProcessorService, pathWatcher *service.PathWatcherAdmin) {
	slog.Info("Running background: ProcessPendingQueue")
	go videoFileProcessorService.ProcessPendingQueue()

	slog.Info("Running background: AddAndWatchPath")
	pathWatcher.AddAndWatchPath(pathWatcher.StreamProcessorConfig)

	slog.Info("Running background: ProcessQueue")
	go videoFileProcessorService.ProcessQueue()
}

// setupHTTPServer configures and returns the main HTTP server for the application. It also starts the Prometheus metrics server on port 2112.
func setupHTTPServer(redisClient *adapter.RedisClientImpl, pathWatcher *service.PathWatcherAdmin) *http.Server {
	v1Handler := &handlers.V1Handler{
		RedisClient: redisClient,
		PathWatcher: pathWatcher,
	}
	handlersRouter := handlers.NewRouter(v1Handler)
	wrappedHandler := middleware.RequestLogger(handlersRouter)
	go func() {
		slog.Info("Starting Prometheus metrics server on :2112/metrics")
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":2112", nil); err != nil {
			slog.Error("Prometheus metrics server error", "err", err)
		}
	}()

	return config.NewHTTPServer(wrappedHandler)
}

// gracefulShutdown gracefully shuts down the HTTP server and closes the Redis client. Logs errors if shutdown fails.
func gracefulShutdown(server *http.Server, redisClient *adapter.RedisClientImpl) {
	slog.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "err", err)
	} else {
		slog.Info("Server exited gracefully")
	}

	if err := redisClient.Close(); err != nil {
		slog.Error("Failed to close Redis client", "err", err)
	} else {
		slog.Info("Redis client closed")
	}

	slog.Info("MinIO client shutdown complete (no explicit close required)")
}

// main is the entry point for the video stream file processor service. It loads configuration, initializes services, runs background tasks, and starts the HTTP server. Handles graceful shutdown on interrupt signals.
func main() {
	// Load configuration
	s3Bucket, defaultPath, streamTimeout, chunkSize, workers := config.LoadEnv()
	streamProcessorConfig := config.StreamProcessorConfig{
		Path:          defaultPath,
		ChunkSize:     chunkSize,
		StreamTimeout: time.Duration(streamTimeout) * time.Second,
		S3Bucket:      s3Bucket,
		NumWorkers:		 uint16(workers),
	}

	// Instantiate external clients
	clients := config.NewAppClients()
	
	// Ensure S3 bucket exists
	ensureS3Bucket(clients.S3Client, streamProcessorConfig.S3Bucket)

	ctx := context.Background()

	// Instantiate services
	pathWatcherAdmin := service.NewPathWatcherAdmin(streamProcessorConfig, clients.RedisClient)
	videoFileProcessorService := service.NewVideoFileProcessorService(ctx, streamProcessorConfig, clients)

	// Run background tasks (sequentially for clarity)
	runBackgroundTasks(videoFileProcessorService, pathWatcherAdmin)

	// Set up HTTP server
	server := setupHTTPServer(clients.RedisClient, pathWatcherAdmin)

	// Channel to listen for interrupt or terminate signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("Starting server", "port", 8080)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "err", err)
		}
	}()

	<-quit
	gracefulShutdown(server, clients.RedisClient)
}
