// Package api exposes a store.Store over HTTP.
package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/edmund1472/raftkv/internal/store"
)

// maxValueBytes caps the size of a PUT body so one request can't use up
// all the server's memory.
const maxValueBytes = 1 << 20 // 1 MiB

// Server translates HTTP requests into store operations.
type Server struct {
	store store.Store
}

// NewServer returns a Server backed by s.
func NewServer(s store.Store) *Server {
	return &Server{store: s}
}

// Handler returns the routes for the key-value API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /kv/{key}", s.handleGet)
	mux.HandleFunc("PUT /kv/{key}", s.handlePut)
	mux.HandleFunc("DELETE /kv/{key}", s.handleDelete)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return mux
}

// handleGet writes the value for the key, or 404 if it doesn't exist.
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	value, err := s.store.Get(r.PathValue("key"))
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, value)
}

// handlePut stores the request body as the value for the key.
func (s *Server) handlePut(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxValueBytes))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "value too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "could not read request body", http.StatusBadRequest)
		return
	}
	if err := s.store.Put(r.PathValue("key"), string(body)); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDelete removes the key. Like the store, it succeeds even if the key
// was already gone.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Delete(r.PathValue("key")); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleHealthz reports that the process is up and serving. Kubernetes polls
// it to decide whether to restart the pod or send it traffic.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "ok")
}
