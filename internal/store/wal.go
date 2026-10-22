package store

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Each log record is laid out like this (integers are little-endian):
//
//	crc    uint32  CRC-32C of every byte after this field
//	op     uint8   opPut or opDelete
//	keyLen uint32
//	valLen uint32
//	key    [keyLen]byte
//	value  [valLen]byte
//
// The checksum is how replay tells a complete record from one that was only
// half-written when the process crashed.
const (
	opPut    byte = 1
	opDelete byte = 2

	headerSize = 4 + 1 + 4 + 4

	// maxRecordBytes caps key+value. Replay uses it to reject a corrupted
	// length field instead of trying to allocate gigabytes.
	maxRecordBytes = 64 << 20 // 64 MiB
)

// ErrTooLarge is returned when a key and value together exceed maxRecordBytes.
var ErrTooLarge = errors.New("key and value too large")

var crcTable = crc32.MakeTable(crc32.Castagnoli)

// WALStore is a MemoryStore made durable by a write-ahead log. Every Put and
// Delete is appended to a file and fsynced before it's applied in memory, and
// the file is replayed on startup to rebuild the map.
type WALStore struct {
	mem *MemoryStore

	mu   sync.Mutex // serializes writers so log order matches apply order
	f    *os.File
	size int64 // length of the valid log; a failed write is cut back to this
	err  error // sticky: once set, every write fails with it
}

// Compile-time check that *WALStore satisfies Store.
var _ Store = (*WALStore)(nil)

// OpenWALStore opens (or creates) the log at path and replays it. A torn or
// corrupt tail, left by a crash mid-write, is logged and truncated away.
func OpenWALStore(path string) (*WALStore, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("wal: open: %w", err)
	}
	// If the file was just created, its directory entry isn't durable until
	// the directory itself is fsynced.
	if err := syncDir(filepath.Dir(path)); err != nil {
		f.Close()
		return nil, err
	}

	mem := NewMemoryStore()
	size, err := replay(f, mem)
	if err != nil {
		f.Close()
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("wal: stat: %w", err)
	}
	if info.Size() > size {
		log.Printf("wal: dropping %d bytes of torn or corrupt data at offset %d in %s",
			info.Size()-size, size, path)
		if err := f.Truncate(size); err != nil {
			f.Close()
			return nil, fmt.Errorf("wal: truncate: %w", err)
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, fmt.Errorf("wal: sync: %w", err)
		}
	}

	return &WALStore{mem: mem, f: f, size: size}, nil
}

// Get reads from memory only. It doesn't take s.mu, so reads never wait
// behind a writer's fsync.
func (s *WALStore) Get(key string) (string, error) {
	return s.mem.Get(key)
}

// Put logs the write, then applies it. It returns only once the write is on disk.
func (s *WALStore) Put(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.appendLocked(opPut, key, value); err != nil {
		return err
	}
	return s.mem.Put(key, value)
}

// Delete logs the delete, then applies it.
func (s *WALStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.appendLocked(opDelete, key, ""); err != nil {
		return err
	}
	return s.mem.Delete(key)
}

// Close closes the log file. Later writes return an error.
func (s *WALStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	s.err = errors.New("wal: store is closed")
	return err
}

// appendLocked writes one record and fsyncs it. The caller must hold s.mu.
func (s *WALStore) appendLocked(op byte, key, value string) error {
	if s.err != nil {
		return s.err
	}
	if len(key)+len(value) > maxRecordBytes {
		// Replay would reject a record this big, so writing it would mean
		// silently losing it (and everything after it) on restart.
		return ErrTooLarge
	}

	rec := encodeRecord(op, key, value)
	if _, err := s.f.Write(rec); err != nil {
		// Part of the record may have reached the file. Cut it off so the next
		// append doesn't land after garbage, which replay would stop at.
		if terr := s.f.Truncate(s.size); terr != nil {
			s.err = fmt.Errorf("wal: write failed and could not be undone: %w", err)
			return s.err
		}
		return fmt.Errorf("wal: write: %w", err)
	}
	if err := s.f.Sync(); err != nil {
		// After a failed fsync the OS may have dropped the dirty pages, so a
		// retry could "succeed" without the data ever reaching disk. The only
		// safe move is to stop accepting writes.
		s.err = fmt.Errorf("wal: fsync failed, refusing further writes: %w", err)
		return s.err
	}
	s.size += int64(len(rec))
	return nil
}

// encodeRecord builds a full record in one buffer so it goes out in a single
// Write call.
func encodeRecord(op byte, key, value string) []byte {
	buf := make([]byte, headerSize+len(key)+len(value))
	buf[4] = op
	binary.LittleEndian.PutUint32(buf[5:9], uint32(len(key)))
	binary.LittleEndian.PutUint32(buf[9:13], uint32(len(value)))
	copy(buf[headerSize:], key)
	copy(buf[headerSize+len(key):], value)
	binary.LittleEndian.PutUint32(buf[0:4], crc32.Checksum(buf[4:], crcTable))
	return buf
}

// replay applies every valid record in r to mem and returns how many bytes
// were valid. It stops at the first short, oversized or checksum-failing
// record; everything from there on is treated as a torn tail.
func replay(r io.Reader, mem *MemoryStore) (int64, error) {
	br := bufio.NewReader(r)
	header := make([]byte, headerSize)
	var good int64

	for {
		if _, err := io.ReadFull(br, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return good, nil
			}
			return 0, fmt.Errorf("wal: read: %w", err)
		}
		crc := binary.LittleEndian.Uint32(header[0:4])
		op := header[4]
		keyLen := binary.LittleEndian.Uint32(header[5:9])
		valLen := binary.LittleEndian.Uint32(header[9:13])
		if uint64(keyLen)+uint64(valLen) > maxRecordBytes {
			return good, nil
		}

		body := make([]byte, keyLen+valLen)
		if _, err := io.ReadFull(br, body); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return good, nil
			}
			return 0, fmt.Errorf("wal: read: %w", err)
		}
		sum := crc32.Update(crc32.Update(0, crcTable, header[4:]), crcTable, body)
		if sum != crc {
			return good, nil
		}

		key, value := string(body[:keyLen]), string(body[keyLen:])
		switch op {
		case opPut:
			mem.Put(key, value)
		case opDelete:
			mem.Delete(key)
		default:
			return good, nil
		}
		good += int64(headerSize + len(body))
	}
}

// syncDir fsyncs a directory so a newly created file in it survives a crash.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("wal: open dir: %w", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("wal: sync dir: %w", err)
	}
	return nil
}
