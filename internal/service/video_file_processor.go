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
	"sync"
	"time"

	"github.com/jonno85/video-stream-file-processor/internal/adapter"
	"github.com/jonno85/video-stream-file-processor/internal/config"
	"github.com/jonno85/video-stream-file-processor/internal/domain"
	"github.com/jonno85/video-stream-file-processor/internal/metrics"
	"github.com/jonno85/video-stream-file-processor/internal/service/utils"
	"github.com/minio/minio-go/v7"
	redis "github.com/redis/go-redis/v9"
)

// VideoFileProcessorService handles processing and uploading video files in chunks.
type VideoFileProcessorService struct {
	ctx                  context.Context
	s3Client             *adapter.S3ClientImpl
	redisClient          *adapter.RedisClientImpl
	streamProcessorConfig config.StreamProcessorConfig
}

// VideoFileProcessor defines the interface for video file processing.
type VideoFileProcessor interface {
	ProcessQueue(streamProcessorConfig config.StreamProcessorConfig) error
	ProcessPendingQueue(streamProcessorConfig config.StreamProcessorConfig) error

	processVideoInQueue(streamProcessorConfig config.StreamProcessorConfig, fileName adapter.FileName) error
	uploadChunks(streamProcessorConfig config.StreamProcessorConfig, file *os.File) error
	uploadChunk(streamProcessorConfig config.StreamProcessorConfig, file *os.File, chunkIndex uint) error
	uploadMetadata(streamProcessorConfig config.StreamProcessorConfig, metadataFile domain.MetadataFile) error
	cleanup(streamProcessorConfig config.StreamProcessorConfig, fileName adapter.FileName) error
}

// NewVideoFileProcessorService creates a new VideoFileProcessorService instance.
func NewVideoFileProcessorService(ctx context.Context, streamProcessorConfig config.StreamProcessorConfig, appClients *config.AppClients) *VideoFileProcessorService {
	return &VideoFileProcessorService{
		s3Client:             appClients.S3Client,
		redisClient:          appClients.RedisClient,
		streamProcessorConfig: streamProcessorConfig,
		ctx:                  ctx,
	}
}

// ProcessQueue starts multiple workers to process all files in the in-progress queue concurrently.
func (vfp *VideoFileProcessorService) ProcessQueue() error {
	var wg sync.WaitGroup
	slog.Info("Starting workers", "numWorkers", vfp.streamProcessorConfig.NumWorkers)
	for i := range make([]struct{}, int(vfp.streamProcessorConfig.NumWorkers)) {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			slog.Info(fmt.Sprintf("Starting worker %d/%d", workerID, vfp.streamProcessorConfig.NumWorkers))
			vfp.processQueueWithDequeueFunc(workerID, vfp.redisClient.DequeueInProgress)
		}(i+1)
	}
	wg.Wait()
	return nil
}

// ProcessPendingQueue processes all files in the stale/pending queue using a single worker.
func (vfp *VideoFileProcessorService) ProcessPendingQueue() error {
	return vfp.processQueueWithDequeueFunc(0, vfp.redisClient.DequeueStaleFile)
}

// processQueueWithDequeueFunc processes files using the provided dequeue function. It loops until no more files are available or an error occurs.
// workerID is used for logging and identifying the worker. dequeueFunc should return the next file to process or redis.Nil when done.
func (vfp *VideoFileProcessorService) processQueueWithDequeueFunc(workerID int, dequeueFunc func(context.Context) (adapter.FileName, error)) error {
	for {
		fileName, err := dequeueFunc(context.Background())
		slog.Info("Dequeued file", "workerID", workerID, "fileName", fileName)
		if err != nil {
			if err == redis.Nil {
				slog.Info("No more videos to process")
				break
			}
			slog.Error("Error dequeuing file", "err", err)
			return err
		}
		if err := vfp.processVideoInQueue(workerID, fileName); err != nil {
			slog.Error("Error processing pending video", "err", err)
			// Continue processing other videos
		}
	}
	return nil
}

// processVideoInQueue processes a single video file by its fileName. It uploads all chunks and records metrics. If all chunks are processed, it dequeues the file.
// Returns an error if processing fails at any step.
func (vfp *VideoFileProcessorService) processVideoInQueue(workerID int, fileName adapter.FileName) error {
	metadata, err := vfp.redisClient.GetMetadataFile(context.Background(), fileName)
	slog.Debug("Metadata", "workerID", workerID, "metadata", metadata)
	if err != nil {
		return err
	}

	if metadata.ChunkProgressIndex >= uint(metadata.TotalChunks) {
		slog.Info("All chunks processed", "workerID", workerID, "fileName", fileName)
		if err := vfp.redisClient.DequeueCompleted(context.Background(), fileName); err != nil {
			return err
		}
		return nil
	}

	file, err := os.Open(metadata.Path)
	if err != nil {
		slog.Error("Failed to open file", "path", metadata.Path, "err", err)
		return err
	}
	defer file.Close()

	if err := vfp.uploadChunks(metadata, file); err != nil {
		slog.Error("Failed to upload chunk", "path", metadata.Path, "err", err)
		return err
	}
	metrics.BytesUploaded.WithLabelValues(metadata.Path).Observe(float64(uint(metadata.TotalBytes)))

	return vfp.cleanup(fileName)
}

// cleanup checks if all chunks for a file are processed and dequeues the file if complete. Returns an error if metadata cannot be retrieved or dequeuing fails.
func (vfp *VideoFileProcessorService) cleanup(fileName adapter.FileName) error {
	metadata, err := vfp.redisClient.GetMetadataFile(context.Background(), fileName)
	if err != nil {
		return err
	}
	if metadata.ChunkProgressIndex >= uint(metadata.TotalChunks) {
		slog.Info("All chunks processed", "fileName", fileName)
		if err := vfp.redisClient.DequeueCompleted(context.Background(), fileName); err != nil {
			return err
		}
		return nil
	}

	slog.Info("Some chunks missing", "fileName", fileName, "metadata", metadata)
	return nil
}

// getChunkData reads the chunk data for the given metadataFiles part from the file. Returns the chunk bytes or an error.
func (vfp *VideoFileProcessorService) getChunkData(metadataFiles domain.MetadataFilePart, file *os.File) ([]byte, error) {
	buffer := make([]byte, metadataFiles.ChunkSize)
	offset := int64(metadataFiles.ChunkIndex) * int64(metadataFiles.ChunkSize)
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		slog.Error("Error seeking file", "err", err)
		return nil, err
	}
	bytesRead, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		slog.Error("Error reading file chunk", "err", err)
		return nil, err
	}
	if err == io.EOF {
		return nil, err
	}
	if bytesRead == 0 {
		slog.Error("No bytes read", "err", err)
		return nil, err
	}
	chunkData := buffer[:bytesRead]
	return chunkData, nil
}

// uploadChunks reads the file in chunks and uploads each chunk to S3. It updates metadata and uploads it to S3 as well. Returns an error if any upload fails.
func (vfp *VideoFileProcessorService) uploadChunks(metadataFiles domain.MetadataFile, file *os.File) error {
	chunkIndex := metadataFiles.ChunkProgressIndex
	totalChunks := uint(metadataFiles.TotalChunks)
	metadataFileInS3 := metadataFiles
	// metadataFileUpdated := metadataFiles
	startTime := time.Now()
	for i := uint(0); i < totalChunks; i++ {
		chunkIndex = metadataFiles.ChunkProgressIndex
		info, err := vfp.uploadChunk(metadataFiles, file, chunkIndex)
		if err != nil {
			if err == io.EOF {
				break
			}
			slog.Error("Error uploading chunk to S3", "err", err)
			metrics.ChunksUploadedErrors.WithLabelValues(metadataFiles.Path).Inc()
			return err
		}
		// if chunk already existed then specialised message
		slog.Debug("Uploaded chunk to S3", "info", info)	
		metrics.ChunksUploaded.WithLabelValues(metadataFiles.Path).Inc()
		metadataFiles.ChunkProgressIndex = chunkIndex + 1
		
		if vfp.redisClient.SetMetadataFile(context.Background(), file.Name(), metadataFiles) != nil {
			slog.Error("Error setting chunk in Redis", "err", err)
			return err
		}
	}

	metadataFiles.StreamDuration = time.Since(startTime)
	metadataFiles.UploadTime = time.Now()
	metadataFiles.ChunkProgressIndex = chunkIndex
	
	if err := vfp.uploadMetadata(metadataFiles); err != nil {
		slog.Error("Error uploading metadata", "err", err)
		metrics.MetadataUploadedErrors.WithLabelValues(metadataFileInS3.Path).Inc()
		return err
	}
	metrics.MetadataUploaded.WithLabelValues(metadataFileInS3.Path).Inc()
	return nil
}

// makeChunkKey generates a chunk key for S3 based on the file path and chunk index. The key includes the file name, chunk index, total chunks, and extension.
func (vfp *VideoFileProcessorService) makeChunkKey(path string, chunkIndex, totalChunks uint) string {
	// Remove folder prefix if present
	parts := strings.Split(path, "/")
	fileName := path
	if len(parts) > 1 {
		fileName = parts[1]
	}
	nameParts := strings.Split(fileName, ".")
	fileNameWithoutExt := strings.Join(nameParts[:len(nameParts)-1], ".")
	ext := nameParts[len(nameParts)-1]
	return fmt.Sprintf("%s_%02d_%02d.%s", fileNameWithoutExt, chunkIndex, totalChunks, ext)
}

// makeChunkMetadata creates the metadata map for a chunk upload, including chunk index, total chunks, path, and hash.
func (vfp *VideoFileProcessorService) makeChunkMetadata(path string, chunkIndex uint, totalChunks uint, hash string) map[string]string {
	return map[string]string{
		"chunkIndex":  fmt.Sprintf("%d", chunkIndex),
		"totalChunks": fmt.Sprintf("%d", totalChunks),
		"path":        path,
		"hash":        hash,
	}
}

// uploadChunk uploads a single chunk to S3 using the provided metadata and file. It computes the hash and updates chunk metadata. Returns the upload info or an error.
func (vfp *VideoFileProcessorService) uploadChunk(metadataFiles domain.MetadataFile, file *os.File, chunkIndex uint) (minio.UploadInfo, error) {
	chunkName := vfp.makeChunkKey(metadataFiles.Path, chunkIndex, metadataFiles.TotalChunks)
	chunkData, err := vfp.getChunkData(metadataFiles.Parts[chunkIndex], file)
	if err != nil {
		return minio.UploadInfo{}, err
	}
	hash, err := utils.ComputeHash(chunkData)
	if err != nil {
		return minio.UploadInfo{}, err
	}
	metadataFiles.Parts[chunkIndex].ChunkName = chunkName
	metadataFiles.Parts[chunkIndex].Hash = hash
	info, err := vfp.s3Client.PutObjectWithIdempotency(context.Background(), metadataFiles.S3Bucket, chunkName, bytes.NewReader(chunkData), hash, int64(len(chunkData)), minio.PutObjectOptions{
		ContentType: "video/mp4",
		UserMetadata: vfp.makeChunkMetadata(metadataFiles.Path, chunkIndex, metadataFiles.TotalChunks, hash),
	})
	return info, err
}

// uploadMetadata uploads the metadata JSON file to S3. Returns an error if marshalling or upload fails.
func (vfp *VideoFileProcessorService) uploadMetadata(metadataFile domain.MetadataFile) error {
	metadataKey := vfp.makeMetadataKey(metadataFile.Path)
	metadataBytes, err := json.Marshal(metadataFile)
	if err != nil {
		slog.Error("Error marshalling metadata", "err", err)
		return err
	}
	metadataHash, err := utils.ComputeHash(metadataBytes)
	if err != nil {
		slog.Error("Error computing metadata hash", "err", err)
		return err
	}
	info, err := vfp.s3Client.PutObjectWithIdempotency(context.Background(), metadataFile.S3Bucket, metadataKey, bytes.NewReader(metadataBytes), metadataHash, int64(len(metadataBytes)), minio.PutObjectOptions{
		ContentType: "application/json",
	})
	if err != nil {
		slog.Error("Error uploading metadata to S3", "err", err)
		return err
	}
	slog.Debug("Uploaded metadata to S3", "metadataKey", metadataKey, "info", info)
	return nil
}

// makeMetadataKey generates the S3 key for the metadata file based on the file path.
func (vfp *VideoFileProcessorService) makeMetadataKey(path string) string {
	parts := strings.Split(path, "/")
	fileName := path
	if len(parts) > 1 {
		fileName = parts[1]
	}
	return fmt.Sprintf("%s_metadata.json", fileName)
}


