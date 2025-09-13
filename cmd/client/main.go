package main

import (
	"log"
	"os"

	"github.com/michael4d45/bizshuffle/internal/client"
)

func main() {
	c, err := client.New(os.Args[1:])
	if err != nil {
		log.Fatalf("client init failed: %v", err)
	}
	c.Run()
	log.Println("[main] client.Run() returned, exiting process")
	os.Exit(0)
}
