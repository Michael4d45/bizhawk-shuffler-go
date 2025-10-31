package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/michael4d45/bizshuffle/internal/client"
)

func main() {
	// Recover from any panics to ensure we log something
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
			debug.PrintStack()
			os.Exit(1)
		}
	}()

	fmt.Fprintf(os.Stderr, "[main] Starting BizShuffle client...\n")

	c, err := client.New(os.Args[1:])
	if err != nil {
		// Log to stderr in case logging hasn't been initialized yet
		fmt.Fprintf(os.Stderr, "ERROR: client init failed: %v\n", err)
		// Also try to log if logging was initialized
		log.Printf("client init failed: %v", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "[main] Client initialized, starting runtime...\n")
	c.Run()
	log.Println("[main] client.Run() returned, exiting process")
	os.Exit(0)
}
