package config

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
)

func LoadEnv() (string, string) {
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
	return s3Bucket, defaultPath
}