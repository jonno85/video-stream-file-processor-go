package config

import (
	"time"

	"github.com/jonno85/video-stream-file-processor.git/clients"
	"github.com/minio/minio-go/v7"
)

type StreamProcessorConfig struct {
	Path string
	S3Bucket string
	ChunkSize uint64
	StreamTimeout time.Duration
}

type AppClients struct {
	RedisClient		*clients.RedisClientImpl
	S3Client			*minio.Client
}

func NewAppClients() *AppClients {
	return &AppClients{
		RedisClient:	clients.NewRedisClientImpl(),
		S3Client:			clients.NewMinioClient(),
	}
}
