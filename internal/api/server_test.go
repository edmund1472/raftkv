package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edmund1472/raftkv/internal/store"
)

// do sends one request straight to the handler (no real network) and returns
// the recorded response.
func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newTestHandler() http.Handler {
	return NewServer(store.NewMemoryStore()).Handler()
}

func TestPutThenGet(t *testing.T) {
	h := newTestHandler()

	if rec := do(h, http.MethodPut, "/kv/user:42", "Alice"); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	rec := do(h, http.MethodGet, "/kv/user:42", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "Alice" {
		t.Errorf("GET body = %q, want %q", got, "Alice")
	}
}

func TestGetMissing(t *testing.T) {
	h := newTestHandler()
	if rec := do(h, http.MethodGet, "/kv/nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET missing key: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDelete(t *testing.T) {
	h := newTestHandler()
	do(h, http.MethodPut, "/kv/k", "v")

	if rec := do(h, http.MethodDelete, "/kv/k", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec := do(h, http.MethodGet, "/kv/k", ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET after DELETE: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	// Deleting a key that doesn't exist is fine (idempotent).
	if rec := do(h, http.MethodDelete, "/kv/k", ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE missing key: status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newTestHandler()
	if rec := do(h, http.MethodPost, "/kv/k", "v"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestPutTooLarge(t *testing.T) {
	h := newTestHandler()
	big := strings.Repeat("x", maxValueBytes+1)
	if rec := do(h, http.MethodPut, "/kv/k", big); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("PUT oversized body: status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}
