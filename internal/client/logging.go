package client

import (
	"io"
	"log"
	"os"
)

// InitLogging sets up global logging and returns the opened log file which the
// caller should Close when finished.
func InitLogging(verbose bool) (*os.File, error) {
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
