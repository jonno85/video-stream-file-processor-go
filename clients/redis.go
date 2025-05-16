package clients

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"time"

	redis "github.com/redis/go-redis/v9"
)

const (
	QueueNew = "queue:new"
	QueueInProgress = "queue:in-progress"
	QueueCompleted = "queue:completed"
	TTL_INFINITE = 0
)

type FileName = string
type FilePath = string

type MetadataFile struct {
	ChunkProgressIndex uint `json:"chunk_progress_index"`
	TotalChunks int `json:"total_chunks"`
	ChunkSize int `json:"chunk_size"`
	Path string `json:"path"`
	UploadTime time.Time `json:"upload_time"`
	StreamDuration time.Duration `json:"stream_duration"`
	S3Bucket string `json:"s3_bucket"`
}

type RedisOperationalClient interface {
	Enqueue(ctx context.Context, key FileName, metadata []MetadataFile) error
	DequeueInProgress(ctx context.Context) (FileName, error)
	DequeueStaleFile(ctx context.Context) (FileName, error)
	DequeueCompleted(ctx context.Context, key FileName) error
	SetPathWatcher(ctx context.Context, key FilePath) error
	GetPathWatcher(ctx context.Context, key FilePath) error
	DelPathWatcher(ctx context.Context, key FilePath) error
	SetMetadataFile(ctx context.Context, key FileName, metadata []MetadataFile) error
	GetMetadataFile(ctx context.Context, key FileName) ([]MetadataFile, error)
	DelMetadataFile(ctx context.Context, key FileName) error
	Close() error
}

type RedisClientImpl struct {
	redisClient *redis.Client
}

func NewRedisClientImpl() *RedisClientImpl {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	password := os.Getenv("REDIS_PASSWORD")
	dbStr := os.Getenv("REDIS_DB")
	db := 0
	if dbStr != "" {
		if parsed, err := strconv.Atoi(dbStr); err == nil {
			db = parsed
		}
	}
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		slog.Error("Failed to connect to Redis", "err", err)
	}
	
	return &RedisClientImpl{
		redisClient: client,
	}
}

func (r *RedisClientImpl) Enqueue(ctx context.Context, key FileName, metadata []MetadataFile) error {
	_, err := r.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		// 1. Push the key to the queue
		if err := pipe.LPush(ctx, QueueNew, key).Err(); err != nil {
			return err
		}
		slog.Info("Enqueued", "key", key)
		jsonBytes, err := json.Marshal(metadata)
		if err != nil {
				return err // handle marshal error
		}
		// 2. Set the metadata
		if err := pipe.Set(ctx, key, jsonBytes, TTL_INFINITE).Err(); err != nil {
			return err
		}
		slog.Info("Set metadata", "key", key)
		return nil
	})
	slog.Info("Enqueued", "key", key, "err", err)
	return err
}

func (r *RedisClientImpl) DequeueInProgress(ctx context.Context) (FileName, error) {
	return r.redisClient.BLMove(ctx, QueueNew, QueueInProgress, "RIGHT", "LEFT", TTL_INFINITE).Result()
}

func (r *RedisClientImpl) DequeueStaleFile(ctx context.Context) (FileName, error) {
	return r.redisClient.LPop(ctx, QueueInProgress).Result()
}

func (r *RedisClientImpl) DequeueCompleted(ctx context.Context, key FileName) error {
	_, err := r.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		slog.Info("DequeueCompleted", "key", key)
		if err := pipe.LRem(ctx, QueueInProgress, 1, key).Err(); err != nil {
			return err
		}
		slog.Info("LRem", "key", key)
		if err := pipe.LPush(ctx, QueueCompleted, key).Err(); err != nil {
			return err
		}
		slog.Info("LPush", "key", key)
		if err := pipe.Del(ctx, key).Err(); err != nil {
			return err
		}
		slog.Info("Del", "key", key)
		return nil
	})

	return err
}

func (r *RedisClientImpl) SetPathWatcher(ctx context.Context, key FilePath) error {
	return r.redisClient.Set(ctx, key, 0, TTL_INFINITE).Err()
}

func (r *RedisClientImpl) GetPathWatcher(ctx context.Context, key FilePath) error {
	return r.redisClient.Get(ctx, key).Err()
}

func (r *RedisClientImpl) DelPathWatcher(ctx context.Context, key FilePath) error {
	return r.redisClient.Del(ctx, key).Err()
}

func (r *RedisClientImpl) SetMetadataFile(ctx context.Context, key FileName, metadata []MetadataFile) error {
	slog.Info("SetMetadataFile", "key", key)
	jsonBytes, err := json.Marshal(metadata)
	if err != nil {
			return err // handle marshal error
	}
	return r.redisClient.Set(ctx, key, jsonBytes, TTL_INFINITE).Err()
}

func (r *RedisClientImpl) GetMetadataFile(ctx context.Context, key FileName) ([]MetadataFile, error) {
	var metadata []MetadataFile
	jsonBytes, err := r.redisClient.Get(ctx, key).Bytes()
	slog.Info("GetMetadataFile", "key", key, "jsonBytes", jsonBytes)
	if err != nil {
		return metadata, err
	}
	
	if err := json.Unmarshal(jsonBytes, &metadata); err != nil {
		return metadata, err
	}
	return metadata, nil
}

func (r *RedisClientImpl) DelMetadataFile(ctx context.Context, key FileName) error {
	return r.redisClient.Del(ctx, key).Err()
}

func (r *RedisClientImpl) Close() error {
	return r.redisClient.Close()
}