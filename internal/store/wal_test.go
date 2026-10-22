package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func openWAL(t *testing.T, path string) *WALStore {
	t.Helper()
	s, err := OpenWALStore(path)
	if err != nil {
		t.Fatalf("OpenWALStore: %v", err)
	}
	return s
}

func wantValue(t *testing.T, s Store, key, want string) {
	t.Helper()
	got, err := s.Get(key)
	if err != nil || got != want {
		t.Errorf("Get(%q) = %q, %v; want %q, nil", key, got, err, want)
	}
}

func wantMissing(t *testing.T, s Store, key string) {
	t.Helper()
	if _, err := s.Get(key); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(%q): err = %v, want ErrNotFound", key, err)
	}
}

// appendBytes simulates a crash that left extra bytes at the end of the log.
func appendBytes(t *testing.T, path string, b []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		t.Fatal(err)
	}
}

func TestWALSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")

	s := openWAL(t, path)
	s.Put("a", "1")
	s.Put("b", "1")
	s.Put("b", "2")
	s.Delete("a")
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s = openWAL(t, path)
	defer s.Close()
	wantMissing(t, s, "a")
	wantValue(t, s, "b", "2")
}

func TestWALTornTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")

	s := openWAL(t, path)
	s.Put("a", "1")
	s.Close()
	sizeBefore := fileSize(t, path)

	// Half a record: the process died partway through a write.
	appendBytes(t, path, encodeRecord(opPut, "b", "2")[:7])

	s = openWAL(t, path)
	wantValue(t, s, "a", "1")
	wantMissing(t, s, "b")
	if got := fileSize(t, path); got != sizeBefore {
		t.Errorf("log size after recovery = %d, want %d (torn tail truncated)", got, sizeBefore)
	}

	// New writes must land after the good data, not after the garbage,
	// or the next replay would stop before reaching them.
	s.Put("c", "3")
	s.Close()

	s = openWAL(t, path)
	defer s.Close()
	wantValue(t, s, "a", "1")
	wantValue(t, s, "c", "3")
}

func TestWALBadChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")

	s := openWAL(t, path)
	s.Put("a", "1")
	s.Close()

	rec := encodeRecord(opPut, "b", "2")
	rec[len(rec)-1] ^= 0xFF // flip bits in the value
	appendBytes(t, path, rec)

	s = openWAL(t, path)
	defer s.Close()
	wantValue(t, s, "a", "1")
	wantMissing(t, s, "b")
}

func TestWALWriteAfterClose(t *testing.T) {
	s := openWAL(t, filepath.Join(t.TempDir(), "wal.log"))
	s.Close()
	if err := s.Put("k", "v"); err == nil {
		t.Error("Put after Close: err = nil, want an error")
	}
}

func TestWALConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")
	s := openWAL(t, path)
	const n = 50

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Put(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i)); err != nil {
				t.Errorf("Put: %v", err)
			}
		}()
	}
	wg.Wait()
	s.Close()

	s = openWAL(t, path)
	defer s.Close()
	for i := 0; i < n; i++ {
		wantValue(t, s, fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}
