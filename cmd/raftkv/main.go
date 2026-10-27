package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/edmund1472/raftkv/internal/api"
	"github.com/edmund1472/raftkv/internal/store"
)

// shutdownTimeout is how long in-flight requests get to finish after a
// SIGTERM. It must stay under Kubernetes' 30s grace period, after which the
// pod is SIGKILLed.
const shutdownTimeout = 10 * time.Second

func main() {
	addr := flag.String("addr", "localhost:8080", "address to listen on")
	dataDir := flag.String("data-dir", "data", "directory for the write-ahead log")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		log.Fatal(err)
	}
	st, err := store.OpenWALStore(filepath.Join(*dataDir, "wal.log"))
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{Addr: *addr, Handler: api.NewServer(st).Handler()}

	// Kubernetes stops a pod with SIGTERM; Ctrl-C sends SIGINT.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	go func() {
		log.Printf("raftkv listening on %s (data in %s)", *addr, *dataDir)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Printf("shutting down")

	// Stop accepting connections and wait for in-flight requests, so no write
	// is cut off between its fsync and its response.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if err := st.Close(); err != nil {
		log.Printf("closing wal: %v", err)
	}
	log.Printf("stopped")
}
