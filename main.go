package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/jonno85/video-stream-file-processor.git/clients"
	"github.com/jonno85/video-stream-file-processor.git/config"
	"github.com/jonno85/video-stream-file-processor.git/handlers"
	"github.com/jonno85/video-stream-file-processor.git/middleware"
	"github.com/jonno85/video-stream-file-processor.git/service"
)

// Ensures the S3 bucket exists, creates it if not
func ensureS3Bucket(s3Client *minio.Client, s3Bucket string) {
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

// Runs the background tasks in sequence
func runBackgroundTasks(videoFileProcessorService *service.VideoFileProcessorService, pathWatcher *service.PathWatcher) {
	slog.Info("Running background: ProcessPendingQueue")
	go videoFileProcessorService.ProcessPendingQueue()

	slog.Info("Running background: AddAndWatchPath")
	pathWatcher.AddAndWatchPath("./input-videos")

	slog.Info("Running background: ProcessQueue")
	go videoFileProcessorService.ProcessQueue()
}

// Sets up and returns the HTTP server
func setupHTTPServer(redisClient *clients.RedisClientImpl, pathWatcher *service.PathWatcher) *http.Server {
	v1Handler := &handlers.V1Handler{
		RedisClient: redisClient,
		PathWatcher: pathWatcher,
	}
	handlersRouter := handlers.NewRouter(v1Handler)
	wrappedHandler := middleware.RequestLogger(handlersRouter)
	return config.NewHTTPServer(wrappedHandler)
}

// Handles graceful shutdown of the server and clients
func gracefulShutdown(server *http.Server, redisClient *clients.RedisClientImpl) {
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

func main() {
	// Load configuration
	s3Bucket, defaultPath := config.LoadEnv()
	streamProcessorConfig := config.StreamProcessorConfig{
		Path:          defaultPath,
		ChunkSize:     config.DEFAULT_CHUNK_SIZE,
		StreamTimeout: 30 * time.Second,
		S3Bucket:      s3Bucket,
	}

	// Instantiate external clients
	clients := config.NewAppClients()
	
	// Ensure S3 bucket exists
	ensureS3Bucket(clients.S3Client, streamProcessorConfig.S3Bucket)

	// Instantiate services
	pathWatcher := service.NewPathWatcher(streamProcessorConfig, clients.RedisClient)
	videoFileProcessorService := service.NewVideoFileProcessorService(streamProcessorConfig, clients)

	// Run background tasks (sequentially for clarity)
	runBackgroundTasks(videoFileProcessorService, pathWatcher)

	// Set up HTTP server
	server := setupHTTPServer(clients.RedisClient, pathWatcher)

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
