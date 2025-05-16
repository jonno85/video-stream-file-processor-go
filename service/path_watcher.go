package service

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jonno85/video-stream-file-processor.git/clients"
	"github.com/jonno85/video-stream-file-processor.git/config"
)

type FsPathWatcher = fsnotify.Watcher

type PathWatcherService struct {
	fsPathWatcher *FsPathWatcher
	logger *slog.Logger
	redisClient *clients.RedisClientImpl
}

// type ProcessorWorker struct {
// 	pathWatcherService *PathWatcherService
// 	// videoFileProcessor *VideoFileProcessor
// }

type PathWatcher struct {
	watchers map[string]*PathWatcherService
	redisClient *clients.RedisClientImpl
	streamProcessorConfig config.StreamProcessorConfig
}

type PathWatcherAction interface {
	AnalyseVideoFile(streamProcessorConfig config.StreamProcessorConfig)
}

func (pw *PathWatcherService) AnalyseVideoFile(streamProcessorConfig config.StreamProcessorConfig) error{
	info, err := os.Stat(streamProcessorConfig.Path)
	if err != nil {
		pw.logger.Error("Failed to stat file", "path", streamProcessorConfig.Path, "err", err)
		return err
	}
	size := info.Size()
	if size == 0 {
		slog.Error("File is empty", "path", streamProcessorConfig.Path)
		return err
	}
	
	file, err := os.Open(streamProcessorConfig.Path)
	if err != nil {
		pw.logger.Error("Failed to open file", "path", streamProcessorConfig.Path, "err", err)
		return err
	}
	defer file.Close()
	
	chunks := size / int64(streamProcessorConfig.ChunkSize)
	slog.Info("Processing video", "path", streamProcessorConfig.Path, "size", size, "chunks", chunks)

	var metadataFiles []clients.MetadataFile
	for i := 0; i <= int(chunks); i++ {
		metadata := clients.MetadataFile{
			ChunkProgressIndex: uint(i),
			TotalChunks:       int(chunks),
			ChunkSize:         int(streamProcessorConfig.ChunkSize),
			Path:              streamProcessorConfig.Path,
			S3Bucket:          streamProcessorConfig.S3Bucket,
		}
		metadataFiles = append(metadataFiles, metadata)
	}
	
	return pw.redisClient.Enqueue(context.Background(), streamProcessorConfig.Path, metadataFiles)
}

func handleWatcherEvents(pathWatcherService *PathWatcherService, done chan bool) {
	go func() {
		for {
			select {
			case event, ok := <-pathWatcherService.fsPathWatcher.Events:
				if !ok {
					return
				}
				slog.Info("event", "action", event.Op, "path", event.Name)
				switch event.Op {
				case fsnotify.Create:
					slog.Info("new file created", "path", event.Name)
					if strings.HasSuffix(event.Name, ".mp4") {
						slog.Info("new video file detected", "path", event.Name)
						
						go pathWatcherService.AnalyseVideoFile(config.StreamProcessorConfig{	
							Path: event.Name,
							S3Bucket: "video-stream-bucket",
							ChunkSize: config.DEFAULT_CHUNK_SIZE,
							StreamTimeout: 30 * time.Second,
						})
					}
				// Verify renaming of existing files
				case fsnotify.Write:
					slog.Info("file updated", "path", event.Name)
				case fsnotify.Remove:
					slog.Info("file removed", "path", event.Name)
				case fsnotify.Rename:
					slog.Info("file renamed", "path", event.Name)
				}
			case err, ok := <-pathWatcherService.fsPathWatcher.Errors:
				if !ok {
					return
				}
				slog.Error("watcher error", "err", err)
			case <-done:
				slog.Info("Shutting down path watcher goroutine")
				return
			}
		}
	}()
}

func (pw *PathWatcher) AddAndWatchPath(path string) {
	fsPathWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("Failed to create pathWatcher", "err", err)
	}
	fsPathWatcher.Add(path)

	pw.redisClient.SetPathWatcher(context.Background(), path)

	// videoFileProcessor := NewVideoFileProcessor(pw.logger, pw.s3Client, pw.redisClient)

	pw.watchers[path] = &PathWatcherService{
		fsPathWatcher: fsPathWatcher,
		redisClient: pw.redisClient,
	}

	done := make(chan bool)

	handleWatcherEvents(pw.watchers[path], done)
}

func NewPathWatcher(streamProcessorConfig config.StreamProcessorConfig, redisClient *clients.RedisClientImpl) *PathWatcher {
	return &PathWatcher{
		watchers: make(map[string]*PathWatcherService),
		redisClient: redisClient,
		streamProcessorConfig: streamProcessorConfig,
	}
}