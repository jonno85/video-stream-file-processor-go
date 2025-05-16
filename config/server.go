package config

import (
	"fmt"
	"net/http"
	"os"
)

// NewHTTPServer creates and returns a configured *http.Server.
// It uses SERVER_PORT env var if set, otherwise defaults to 8080.
func NewHTTPServer(handler http.Handler) *http.Server {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	addr := fmt.Sprintf(":%s", port)
	return &http.Server{
		Addr:    addr,
		Handler: handler,
	}
} 