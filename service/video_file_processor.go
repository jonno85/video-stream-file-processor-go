package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jonno85/video-stream-file-processor.git/clients"
	"github.com/jonno85/video-stream-file-processor.git/config"
	"github.com/minio/minio-go/v7"
	redis "github.com/redis/go-redis/v9"
)

// VideoFileProcessorService handles processing and uploading video files in chunks.
type VideoFileProcessorService struct {
	s3Client             *minio.Client
	redisClient          *clients.RedisClientImpl
	streamProcessorConfig config.StreamProcessorConfig
}

// VideoFileProcessor defines the interface for video file processing.
type VideoFileProcessor interface {
	ProcessVideo(streamProcessorConfig config.StreamProcessorConfig) error
	ProcessQueue(streamProcessorConfig config.StreamProcessorConfig) error
	ProcessPendingQueue(streamProcessorConfig config.StreamProcessorConfig) error
	processPendingVideo(streamProcessorConfig config.StreamProcessorConfig, fileName clients.FileName) error
	uploadChunks(streamProcessorConfig config.StreamProcessorConfig, file *os.File) error
	uploadMetadata(streamProcessorConfig config.StreamProcessorConfig, metadataFile clients.MetadataFile) error
	cleanup(streamProcessorConfig config.StreamProcessorConfig, fileName clients.FileName) error
}

// NewVideoFileProcessorService creates a new VideoFileProcessorService instance.
func NewVideoFileProcessorService(streamProcessorConfig config.StreamProcessorConfig, appClients *config.AppClients) *VideoFileProcessorService {
	return &VideoFileProcessorService{
		s3Client:             appClients.S3Client,
		redisClient:          appClients.RedisClient,
		streamProcessorConfig: streamProcessorConfig,
	}
}

// ProcessQueue processes all files in the in-progress queue.
func (vfp *VideoFileProcessorService) ProcessQueue() error {
	return vfp.processQueueWithDequeueFunc(vfp.redisClient.DequeueInProgress)
}

// ProcessPendingQueue processes all files in the stale/pending queue.
func (vfp *VideoFileProcessorService) ProcessPendingQueue() error {
	return vfp.processQueueWithDequeueFunc(vfp.redisClient.DequeueStaleFile)
}

// processQueueWithDequeueFunc is a helper to process files using a given dequeue function.
func (vfp *VideoFileProcessorService) processQueueWithDequeueFunc(dequeueFunc func(context.Context) (clients.FileName, error)) error {
	for {
		fileName, err := dequeueFunc(context.Background())
		slog.Info("Dequeued file", "fileName", fileName)
		if err != nil {
			if err == redis.Nil {
				slog.Info("No more videos to process")
				break
			}
			slog.Error("Error dequeuing file", "err", err)
			return err
		}
		if err := vfp.processPendingVideo(fileName); err != nil {
			slog.Error("Error processing pending video", "err", err)
			// Continue processing other videos
		}
	}
	return nil
}

// processPendingVideo processes a single video file by fileName.
func (vfp *VideoFileProcessorService) processPendingVideo(fileName clients.FileName) error {
	metadata, err := vfp.redisClient.GetMetadataFile(context.Background(), fileName)
	slog.Info("Metadata", "metadata", metadata)
	if err != nil {
		return err
	}

	if len(metadata) == 0 || metadata[0].ChunkProgressIndex >= uint(metadata[0].TotalChunks) {
		slog.Info("All chunks processed", "fileName", fileName)
		if err := vfp.redisClient.DequeueCompleted(context.Background(), fileName); err != nil {
			return err
		}
		return nil
	}

	file, err := os.Open(metadata[0].Path)
	if err != nil {
		slog.Error("Failed to open file", "path", metadata[0].Path, "err", err)
		return err
	}
	defer file.Close()

	if err := vfp.uploadChunks(metadata, file); err != nil {
		slog.Error("Failed to upload chunk", "path", metadata[0].Path, "err", err)
		return err
	}

	return vfp.cleanup(fileName)
}

// cleanup checks if all chunks are processed and dequeues the file if complete.
func (vfp *VideoFileProcessorService) cleanup(fileName clients.FileName) error {
	metadata, err := vfp.redisClient.GetMetadataFile(context.Background(), fileName)
	if err != nil {
		return err
	}
	if len(metadata) >= int(metadata[0].TotalChunks) {
		slog.Info("All chunks processed", "fileName", fileName)
		if err := vfp.redisClient.DequeueCompleted(context.Background(), fileName); err != nil {
			return err
		}
		return nil
	}

	slog.Info("Some chunks missing", "fileName", fileName, "metadata", metadata)
	return nil
}

// uploadChunks reads the file in chunks and uploads each chunk to S3.
func (vfp *VideoFileProcessorService) uploadChunks(metadataFiles []clients.MetadataFile, file *os.File) error {
	buffer := make([]byte, metadataFiles[0].ChunkSize)
	startTime := time.Now()
	chunkIndex := metadataFiles[0].ChunkProgressIndex
	totalChunks := uint(metadataFiles[0].TotalChunks)
	metadataFileInS3 := metadataFiles[0]
	for ; chunkIndex <= totalChunks; chunkIndex++ {
		bytesRead, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			slog.Error("Error reading file chunk", "err", err)
			return err
		}
		if err == io.EOF {
			break
		}
		if bytesRead == 0 {
			slog.Error("No bytes read", "err", err)
			break
		}
		chunkData := buffer[:bytesRead]
		chunkKey := vfp.makeChunkKey(metadataFiles[chunkIndex].Path, chunkIndex, totalChunks)
		metadata := vfp.makeChunkMetadata(metadataFiles[chunkIndex], chunkIndex)
		// upload data to s3
		info, err := vfp.s3Client.PutObject(context.Background(), metadataFiles[chunkIndex].S3Bucket, chunkKey, bytes.NewReader(chunkData), int64(bytesRead), minio.PutObjectOptions{
			ContentType:  "video/mp4",
			UserMetadata: metadata,
		})
		if err != nil {
			slog.Error("Error uploading chunk to S3", "err", err)
			return err
		}
		slog.Info("Uploaded chunk to S3", "chunkKey", chunkKey, "info", info)
		metadataFileUpdated := metadataFiles[1:]
		
		if vfp.redisClient.SetMetadataFile(context.Background(), file.Name(), metadataFileUpdated) != nil {
			slog.Error("Error setting chunk in Redis", "err", err)
			return err
		}
		slog.Info("Processed chunk", "chunkIndex", chunkIndex, "bytesRead", bytesRead)
	}
	metadataFileInS3.StreamDuration = time.Since(startTime)
	metadataFileInS3.UploadTime = time.Now()
	metadataFileInS3.ChunkProgressIndex = chunkIndex
	if err := vfp.uploadMetadata(metadataFileInS3); err != nil {
		slog.Error("Error uploading metadata", "err", err)
		return err
	}
	return nil
}

// makeChunkKey generates a chunk key for S3 based on the file path and chunk index.
func (vfp *VideoFileProcessorService) makeChunkKey(path string, chunkIndex, totalChunks uint) string {
	// Remove folder prefix if present
	parts := strings.Split(path, "/")
	fileName := path
	if len(parts) > 1 {
		fileName = parts[1]
	}
	return fmt.Sprintf("%s_%d_%d", fileName, chunkIndex, totalChunks)
}

// makeChunkMetadata creates the metadata map for a chunk upload.
func (vfp *VideoFileProcessorService) makeChunkMetadata(metadataFile clients.MetadataFile, chunkIndex uint) map[string]string {
	return map[string]string{
		"chunkIndex":  fmt.Sprintf("%d", chunkIndex),
		"totalChunks": fmt.Sprintf("%d", metadataFile.TotalChunks),
		"path":        metadataFile.Path,
	}
}

// uploadMetadata uploads the metadata JSON file to S3.
func (vfp *VideoFileProcessorService) uploadMetadata(metadataFile clients.MetadataFile) error {
	metadataKey := vfp.makeMetadataKey(metadataFile.Path)
	metadataBytes, err := json.Marshal(metadataFile)
	if err != nil {
		slog.Error("Error marshalling metadata", "err", err)
		return err
	}
	info, err := vfp.s3Client.PutObject(context.Background(), metadataFile.S3Bucket, metadataKey, bytes.NewReader(metadataBytes), int64(len(metadataBytes)), minio.PutObjectOptions{
		ContentType: "application/json",
	})
	if err != nil {
		slog.Error("Error uploading metadata to S3", "err", err)
		return err
	}
	slog.Info("Uploaded metadata to S3", "metadataKey", metadataKey, "info", info)
	return nil
}

// makeMetadataKey generates the S3 key for the metadata file.
func (vfp *VideoFileProcessorService) makeMetadataKey(path string) string {
	parts := strings.Split(path, "/")
	fileName := path
	if len(parts) > 1 {
		fileName = parts[1]
	}
	return fmt.Sprintf("%s_metadata", fileName)
}


