package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jonno85/video-stream-file-processor/internal/adapter"
	"github.com/jonno85/video-stream-file-processor/internal/config"
	"github.com/jonno85/video-stream-file-processor/internal/service"
)

type ProcessRequest struct {
	Path string `json:"path"`
	ChunkSize int `json:"chunk_size,omitempty"`
}

type V1Handler struct {
	RedisClient *adapter.RedisClientImpl
	PathWatcher *service.PathWatcherAdmin
}

func (h *V1Handler) AddPathToWatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var request ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if request.Path == "" {
		http.Error(w, "Path is required", http.StatusBadRequest)
		return
	}

	if request.ChunkSize == 0 {
		request.ChunkSize = config.DEFAULT_CHUNK_SIZE
	}
	
	if h.RedisClient.GetPathWatcher(r.Context(), request.Path) == nil {
		http.Error(w, "Path is already added", http.StatusConflict)
		return
	}
	
	streamProcessorConfig := h.PathWatcher.StreamProcessorConfig
	streamProcessorConfig.Path = request.Path
	streamProcessorConfig.ChunkSize = uint64(request.ChunkSize)
	
	h.PathWatcher.AddAndWatchPath(streamProcessorConfig)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Path added to watchlist"))
}

func (h *V1Handler) RemovePathFromWatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.PathWatcher.DeleteWatchPath(request.Path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	w.Write([]byte("Path removed from watchlist"))
}