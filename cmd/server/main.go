package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/michael4d45/bizshuffle/internal/server"
)

func main() {
	host := flag.String("host", "127.0.0.1", "host to bind")
	port := flag.Int("port", 8080, "port to bind")
	flag.Parse()

	root, _ := os.Getwd()
	stateFile := filepath.Join(root, "state.json")
	s := server.New(stateFile)

	chosenHost := *host
	if chosenHost == "127.0.0.1" {
		if persisted := s.PersistedHost(); persisted != "" {
			chosenHost = persisted
		}
	}
	s.UpdateHostIfChanged(chosenHost)

	addr := fmt.Sprintf("%s:%d", chosenHost, *port)
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	log.Printf("Starting server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
