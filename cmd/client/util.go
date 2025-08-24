package main

import (
	"errors"
	"io"
	"log"
	"os"
)

// ErrNotFound is returned when a requested remote save/file is not present on the server
var ErrNotFound = errors.New("not found")

func initLogging(verbose bool) (*os.File, error) {
	f, err := os.OpenFile("client.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		return nil, err
	}
	if verbose {
		mw := io.MultiWriter(os.Stdout, f)
		log.SetOutput(mw)
	} else {
		log.SetOutput(f)
	}
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	return f, nil
}
