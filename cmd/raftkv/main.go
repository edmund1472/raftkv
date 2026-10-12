package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/edmund1472/raftkv/internal/api"
	"github.com/edmund1472/raftkv/internal/store"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "address to listen on")
	flag.Parse()

	srv := api.NewServer(store.NewMemoryStore())

	log.Printf("raftkv listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, srv.Handler()))
}
