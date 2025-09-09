package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/michael4d45/bizshuffle/internal/server"
)

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
	log.Fatal(http.ListenAndServe(addr, mux))
}
