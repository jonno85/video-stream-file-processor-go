package config

import (
	"log/slog"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

func LoadEnv() (string, string, int, uint64, int) {
	err := godotenv.Load()
	if err != nil {
		slog.Error("No .env file found or error loading .env file", "err", err)
	}
	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		slog.Error("S3_BUCKET is not set")
		os.Exit(1)
	}
	defaultPath := os.Getenv("DEFAULT_INPUT_PATH")
	if defaultPath == "" {
		slog.Error("DEFAULT_INPUT_PATH is not set")
		os.Exit(1)
	}
	streamTimeout := os.Getenv("STREAM_TIMEOUT_SEC")
	if streamTimeout == "" {
		slog.Warn("STREAM_TIMEOUT_SEC is not set, using default of 30 seconds")
		streamTimeout = "30"
	}
	timeout, err := strconv.Atoi(streamTimeout)
	if err != nil {
		slog.Error("STREAM_TIMEOUT_SEC is not a valid integer", "err", err)
		os.Exit(1)
	}
	chunkSize := os.Getenv("CHUNK_SIZE")
	if chunkSize == "" {
		slog.Warn("CHUNK_SIZE is not set, using default of 1024 bytes")
		chunkSize = strconv.Itoa(DEFAULT_CHUNK_SIZE)
	}
	chunkSizeInt, err := strconv.Atoi(chunkSize)
	if err != nil {
		slog.Error("CHUNK_SIZE is not a valid integer", "err", err)
		os.Exit(1)
	}
	workers := os.Getenv("NUM_WORKERS")
	if workers == "" {
		slog.Warn("WORKERS is not set, using default of 1")
		workers = "1"
	}
	workersInt, _ := strconv.Atoi(workers)
	return s3Bucket, defaultPath, timeout, uint64(chunkSizeInt), workersInt
}	