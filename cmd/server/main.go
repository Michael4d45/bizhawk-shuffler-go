package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/michael4d45/bizshuffle/clienthost"
	"github.com/michael4d45/bizshuffle/serverhost"
)

func main() {
	defaultDir, err := clienthost.DefaultDataDir()
	if err != nil {
		log.Fatal(err)
	}
	dataDir := flag.String("data-dir", defaultDir, "server data directory")
	host := flag.String("host", "0.0.0.0", "host to bind")
	port := flag.Int("port", 8080, "port to bind")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.Chdir(*dataDir); err != nil {
		log.Fatal(err)
	}

	s := serverhost.New()
	chosenHost := *host
	if chosenHost == "127.0.0.1" {
		if persisted := s.PersistedHost(); persisted != "" {
			chosenHost = persisted
		}
	}
	s.SetHost(chosenHost)
	chosenPort := *port
	if chosenPort == 8080 {
		if persisted := s.PersistedPort(); persisted != 0 {
			chosenPort = persisted
		}
	}
	s.SetPort(chosenPort)

	addr := fmt.Sprintf("%s:%d", chosenHost, chosenPort)
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		log.Printf("BizShuffle server listening at http://%s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	_ = srv.Close()
}
