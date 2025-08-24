package main

import (
	"log"
	"os"

	"github.com/michael4d45/bizshuffle/internal/client"
)

func main() {
	if err := client.Run(os.Args[1:]); err != nil {
		log.Fatalf("client failed: %v", err)
	}
}
