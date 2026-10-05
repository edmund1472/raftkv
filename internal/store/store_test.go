package store

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestPutThenGet(t *testing.T) {
	s := NewMemoryStore()
	if err := s.Put("user:42", "Alice"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get("user:42")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "Alice" {
		t.Errorf("Get = %q, want %q", got, "Alice")
	}
}

func TestGetMissing(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Get("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing key: err = %v, want ErrNotFound", err)
	}
}

func TestOverwrite(t *testing.T) {
	s := NewMemoryStore()
	s.Put("k", "v1")
	s.Put("k", "v2")
	got, err := s.Get("k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "v2" {
		t.Errorf("Get = %q, want %q", got, "v2")
	}
}

func TestDelete(t *testing.T) {
	s := NewMemoryStore()
	s.Put("k", "v")
	if err := s.Delete("k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
	// Deleting a key that doesn't exist is fine (idempotent).
	if err := s.Delete("k"); err != nil {
		t.Errorf("Delete missing key: err = %v, want nil", err)
	}
}

func TestConcurrentWrites(t *testing.T) {
	s := NewMemoryStore()
	const n = 100

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Put(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
		}()
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		key, want := fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i)
		got, err := s.Get(key)
		if err != nil || got != want {
			t.Errorf("Get(%q) = %q, %v; want %q, nil", key, got, err, want)
		}
	}
}
