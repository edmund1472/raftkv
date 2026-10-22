package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/edmund1472/raftkv/internal/api"
	"github.com/edmund1472/raftkv/internal/store"
)

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

	srv := api.NewServer(st)

	log.Printf("raftkv listening on %s (data in %s)", *addr, *dataDir)
	log.Fatal(http.ListenAndServe(*addr, srv.Handler()))
}
