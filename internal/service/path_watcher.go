package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jonno85/video-stream-file-processor/internal/adapter"
	"github.com/jonno85/video-stream-file-processor/internal/config"
	"github.com/jonno85/video-stream-file-processor/internal/domain"
	metrics "github.com/jonno85/video-stream-file-processor/internal/metrics"
)

// FsPathWatcher is an alias for fsnotify.Watcher, used for file system event watching.
type FsPathWatcher = fsnotify.Watcher

// PathWatcherService provides methods to watch a file system path and enqueue video files for processing.
type PathWatcherService struct {
	fsPathWatcher *FsPathWatcher
	redisClient adapter.RedisOperationalClient
	done chan bool
}

type PathWatcherAdminAction interface {
	AddAndWatchPath(streamProcessorConfig config.StreamProcessorConfig)
	DeleteWatchPath(path string) error
}

// PathWatcher manages multiple PathWatcherService instances and coordinates file watching and processing.
type PathWatcherAdmin struct {
	watchers map[string]*PathWatcherService
	redisClient adapter.RedisOperationalClient
	StreamProcessorConfig config.StreamProcessorConfig
}

var fileTimers = make(map[string]*time.Timer)
var mu sync.Mutex

// PathWatcherServiceAction defines the interface for actions that can be performed by a path watcher, such as analyzing a video file or starting/resetting a timer.
type PathWatcherServiceAction interface {
	analyseVideoFile(streamProcessorConfig config.StreamProcessorConfig)
	startOrResetTimer(streamProcessorConfig config.StreamProcessorConfig)
}

// analyseVideoFile analyzes the video file at the path specified in streamProcessorConfig. It checks file size, splits it into chunks, creates metadata, and enqueues it for processing. Returns an error if the file cannot be processed.
func (pw *PathWatcherService) analyseVideoFile(streamProcessorConfig config.StreamProcessorConfig) error{
	info, err := os.Stat(streamProcessorConfig.Path)
	if err != nil {
		slog.Error("Failed to stat file", "path", streamProcessorConfig.Path, "err", err)
		metrics.FilesIngestedErrors.WithLabelValues(streamProcessorConfig.Path).Inc()
		return err
	}
	size := info.Size()
	if size == 0 {
		slog.Error("File is empty", "path", streamProcessorConfig.Path)
		metrics.FilesIngestedErrors.WithLabelValues(streamProcessorConfig.Path).Inc()
		return err
	}
	
	file, err := os.Open(streamProcessorConfig.Path)
	if err != nil {
		slog.Error("Failed to open file", "path", streamProcessorConfig.Path, "err", err)
		metrics.FilesIngestedErrors.WithLabelValues(streamProcessorConfig.Path).Inc()
		return err
	}
	defer file.Close()
	
	chunks := size / int64(streamProcessorConfig.ChunkSize)
	if size % int64(streamProcessorConfig.ChunkSize) != 0 {
		chunks++
	}
	slog.Info("Processing video", "path", streamProcessorConfig.Path, "size", size, "chunks", chunks)

	var metadata domain.MetadataFile
	metadata.ChunkProgressIndex = 0
	metadata.TotalChunks = uint(chunks)
	metadata.TotalBytes = uint64(size)
	metadata.Path = streamProcessorConfig.Path
	metadata.S3Bucket = streamProcessorConfig.S3Bucket
	metadata.Parts = make([]domain.MetadataFilePart, chunks)
	for i := range make([]struct{}, chunks) {
		metadataPart := domain.MetadataFilePart{
					ChunkIndex: uint(i),
					ChunkSize: int(streamProcessorConfig.ChunkSize),
				}
		metadata.Parts[i] = metadataPart
	}
	
	err = pw.redisClient.Enqueue(context.Background(), streamProcessorConfig.Path, metadata)
	if err != nil {
		slog.Error("Failed to enqueue video file", "path", streamProcessorConfig.Path, "err", err)
		metrics.FilesIngestedErrors.WithLabelValues(streamProcessorConfig.Path).Inc()
		return err
	}
	metrics.FilesIngested.WithLabelValues(streamProcessorConfig.Path).Inc()
	return nil
}

// startOrResetTimer starts or resets a timer for the given file path. When the timer expires, it triggers analyseVideoFile for the file. Used to debounce file events.
func (pw *PathWatcherService) startOrResetTimer(streamProcessorConfig config.StreamProcessorConfig) {
	filePath := streamProcessorConfig.Path
	mu.Lock()
	defer mu.Unlock()
	if timer, exists := fileTimers[filePath]; exists {
		timer.Stop()
	}

	fileTimers[filePath] = time.AfterFunc(streamProcessorConfig.StreamTimeout, func() {
		slog.Info("No updates, processing file", "timeout", streamProcessorConfig.StreamTimeout.String(), "path", filePath)
		go pw.analyseVideoFile(streamProcessorConfig)
		// Clean up
		mu.Lock()
		delete(fileTimers, filePath)
		mu.Unlock()
	})
}

// handleWatcherEvents listens for file system events and triggers appropriate actions (such as starting/resetting timers or analyzing files) based on the event type. Runs in a goroutine and exits when the done channel is closed.
func handleWatcherEvents(pathWatcherService *PathWatcherService, streamProcessorConfig config.StreamProcessorConfig, done chan bool) {
	go func() {
		for {
			select {
			case event, ok := <-pathWatcherService.fsPathWatcher.Events:
				if !ok {
					return
				}
				slog.Debug("event", "action", event.Op, "path", event.Name)
				switch event.Op {
				case fsnotify.Create:
					slog.Debug("new file created", "path", event.Name)
					if strings.HasSuffix(event.Name, ".mp4") {
						streamProcessorConfig.Path = event.Name
						pathWatcherService.startOrResetTimer(streamProcessorConfig)
					}
				// Verify renaming of existing files
				case fsnotify.Write:
					slog.Debug("file updated", "path", event.Name)
					if strings.HasSuffix(event.Name, ".mp4") {
						streamProcessorConfig.Path = event.Name
						pathWatcherService.startOrResetTimer(streamProcessorConfig)
					}
				case fsnotify.Remove, fsnotify.Rename:
					slog.Debug("file renamed/removed", "path", event.Name)
				case fsnotify.Chmod:
					slog.Debug("file chmod", "path", event.Name)
				default:
					slog.Debug("unknown event", "path", event.Name, "action", event.Op.String())
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
	slog.Info("Watcher events exiting goroutine")
}

// AddAndWatchPath adds a new path to be watched for file system events and starts handling events for that path. Registers the watcher in Redis and starts the event handler goroutine.
func (pw *PathWatcherAdmin) AddAndWatchPath(streamProcessorConfig config.StreamProcessorConfig) {
	path := streamProcessorConfig.Path
	fsPathWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("Failed to create pathWatcher", "err", err)
	}
	fsPathWatcher.Add(path)

	pw.redisClient.SetPathWatcher(context.Background(), path)
	slog.Info("Path added to watchlist", "path", path)
	
	done := make(chan bool)
	pw.watchers[path] = &PathWatcherService{
		fsPathWatcher: fsPathWatcher,
		redisClient: pw.redisClient,
		done: done,
	}

	slog.Debug("Starting watcher events", "path", path, "streamProcessorConfig", streamProcessorConfig, "watchers", pw.watchers)
	handleWatcherEvents(pw.watchers[path], streamProcessorConfig, done)
}

// DeleteWatchPath deletes a path from the watchlist and stops watching it. Returns an error if the path is not found.
func (pw *PathWatcherAdmin) DeleteWatchPath(path string) error {
	slog.Info("Deleting watch path", "path", path)

	if pw.redisClient.GetPathWatcher(context.Background(), path) == nil {
		return errors.New("path not found")
	}

	if watcherService, ok := pw.watchers[path]; ok {
		close(watcherService.done)
		watcherService.fsPathWatcher.Close()
		delete(pw.watchers, path)
		pw.redisClient.DelPathWatcher(context.Background(), path)
		slog.Info("Path removed from watchlist", "path", path)
		return nil
	}

	return errors.New("path not found")
}

// NewPathWatcher creates a new PathWatcher instance with the provided streamProcessorConfig and redisClient. Initializes the watchers map.
func NewPathWatcherAdmin(streamProcessorConfig config.StreamProcessorConfig, redisClient adapter.RedisOperationalClient) *PathWatcherAdmin {
	return &PathWatcherAdmin{
		watchers: make(map[string]*PathWatcherService),
		redisClient: redisClient,
		StreamProcessorConfig: streamProcessorConfig,
	}
}