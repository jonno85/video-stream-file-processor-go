package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jonno85/video-stream-file-processor.git/clients"
	"github.com/jonno85/video-stream-file-processor.git/config"
	"github.com/jonno85/video-stream-file-processor.git/service"
)

type ProcessRequest struct {
	Path string `json:"path"`
	ChunkSize int `json:"chunk_size,omitempty"`
}

type V1Handler struct {
	RedisClient *clients.RedisClientImpl
	PathWatcher *service.PathWatcher
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
	
	h.PathWatcher.AddAndWatchPath(request.Path)

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

	if h.RedisClient.GetPathWatcher(r.Context(), request.Path) != nil {
		http.Error(w, "Path is not available added", http.StatusNotFound)
		return
	}

	h.RedisClient.DelPathWatcher(r.Context(), request.Path)

	w.WriteHeader(http.StatusNoContent)
	w.Write([]byte("Path removed from watchlist"))
}