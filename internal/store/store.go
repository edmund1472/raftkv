// Package store holds the storage engines for raftkv.
package store

import (
	"errors"
	"sync"
)

// ErrNotFound is returned when a key doesn't exist.
var ErrNotFound = errors.New("key not found")

// Store is the interface every storage engine implements.
// MemoryStore implements it today; the WAL-backed store will too.
type Store interface {
	Get(key string) (string, error)
	Put(key, value string) error
	Delete(key string) error
}

// MemoryStore keeps everything in a map, guarded by a read/write lock.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]string
}

// Compile-time check that *MemoryStore satisfies Store.
var _ Store = (*MemoryStore)(nil)

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]string)}
}

// Get returns the value for key, or ErrNotFound.
func (s *MemoryStore) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Put sets key to value, overwriting any previous value.
func (s *MemoryStore) Put(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = value
	return nil
}

// Delete removes key. Deleting a missing key is not an error, so replaying
// the same delete twice (from a log) is safe.
func (s *MemoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, key)
	return nil
}
