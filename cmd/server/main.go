package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/michael4d45/bizshuffle/internal/server"
)

// TODO: Integrate discovery broadcaster with server lifecycle
// - Start broadcaster after server begins listening
// - Stop broadcaster before server exits
// - Pass server info (host, port) to broadcaster
// - Handle broadcaster startup/shutdown errors gracefully

func main() {
	host := flag.String("host", "127.0.0.1", "host to bind")
	port := flag.Int("port", 8080, "port to bind")
	flag.Parse()

	s := server.New()

	chosenHost := *host
	if chosenHost == "127.0.0.1" {
		if persisted := s.PersistedHost(); persisted != "" {
			chosenHost = persisted
		}
	}
	s.UpdateHostIfChanged(chosenHost)

	chosenPort := *port
	if chosenPort == 8080 {
		if persisted := s.PersistedPort(); persisted != 0 {
			chosenPort = persisted
		}
	}
	s.UpdatePortIfChanged(chosenPort)

	addr := fmt.Sprintf("%s:%d", chosenHost, chosenPort)
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	protocol := "http"
	if chosenPort == 443 || chosenPort == 8443 {
		protocol = "https"
	}
	log.Printf("Starting server on %s://%s", protocol, addr)
	// TODO: Start discovery broadcaster
	if err := s.StartBroadcaster(context.Background()); err != nil {
		log.Printf("Failed to start discovery broadcaster: %v", err)
	}
	log.Fatal(http.ListenAndServe(addr, mux))
}
