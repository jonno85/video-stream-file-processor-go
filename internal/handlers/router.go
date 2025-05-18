package handlers

import "net/http"

func NewRouter(v1Handler *V1Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", HealthCheck)
	mux.HandleFunc("/v1/path/add", v1Handler.AddPathToWatch)
	mux.HandleFunc("/v1/path/remove", v1Handler.RemovePathFromWatch)
	return mux
}

