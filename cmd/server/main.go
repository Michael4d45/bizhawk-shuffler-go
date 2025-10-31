package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/michael4d45/bizshuffle/internal/server"
)

// openBrowser opens the default browser to the specified URL
func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("cmd", "/c", "start", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	default:
		log.Printf("Unsupported platform to open browser: %s", runtime.GOOS)
		return
	}
	if err != nil {
		log.Printf("Failed to open browser: %v", err)
	} else {
		log.Printf("Opened browser to %s", url)
	}
}

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
	protocol := "http"
	if chosenPort == 443 || chosenPort == 8443 {
		protocol = "https"
	}
	url := fmt.Sprintf("%s://%s", protocol, addr)
	log.Printf("Starting server on %s", url)
	if err := s.StartBroadcaster(context.Background()); err != nil {
		log.Printf("Failed to start discovery broadcaster: %v", err)
	}

	// Open browser after a short delay to ensure server is ready
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(url)
	}()

	log.Fatal(http.ListenAndServe(addr, mux))
}
