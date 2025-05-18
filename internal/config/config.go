package config

import (
	"time"

	"github.com/jonno85/video-stream-file-processor/internal/adapter"
)

type StreamProcessorConfig struct {
	Path string
	S3Bucket string
	ChunkSize uint64
	StreamTimeout time.Duration
	NumWorkers uint16
}

type AppClients struct {
	RedisClient		*adapter.RedisClientImpl
	S3Client			*adapter.S3ClientImpl
}

func NewAppClients() *AppClients {
	return &AppClients{
		RedisClient:	adapter.NewRedisClientImpl(),
		S3Client:			adapter.NewMinioClient(),
	}
}
