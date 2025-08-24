package client

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// BuildWSAndHTTP takes the -server flag value (which may be a ws:// or http://
// form) and the stored config and returns a websocket URL to connect to and a
// corresponding http(s) base URL for REST calls. It mirrors the URL logic
// previously in run.go so the construction can be reused and tested.
func BuildWSAndHTTP(serverFlag string, cfg Config) (wsURL string, serverHTTP string, err error) {
	serverHTTP = ""
	if s, ok := cfg["server"]; ok && s != "" {
		serverHTTP = s
	}

	if strings.HasPrefix(serverFlag, "ws://") || strings.HasPrefix(serverFlag, "wss://") {
		u, err := url.Parse(serverFlag)
		if err != nil {
			return "", "", fmt.Errorf("invalid server url %q: %w", serverFlag, err)
		}
		if u.Path == "" || u.Path == "/" {
			u.Path = "/ws"
		} else if !strings.HasSuffix(u.Path, "/ws") {
			u.Path = strings.TrimRight(u.Path, "/") + "/ws"
		}
		wsURL = u.String()
		if serverHTTP == "" {
			hu := *u
			switch hu.Scheme {
			case "ws":
				hu.Scheme = "http"
			case "wss":
				hu.Scheme = "https"
			}
			hu.Path = ""
			hu.RawQuery = ""
			hu.Fragment = ""
			serverHTTP = hu.String()
		}
		return wsURL, serverHTTP, nil
	}

	if serverHTTP == "" {
		return "", "", fmt.Errorf("no server configured for websocket and -server flag not provided")
	}
	hu, err := url.Parse(serverHTTP)
	if err != nil {
		return "", "", fmt.Errorf("invalid configured server %q: %w", serverHTTP, err)
	}
	switch hu.Scheme {
	case "http":
		hu.Scheme = "ws"
	case "https":
		hu.Scheme = "wss"
	}
	if hu.Path == "" || hu.Path == "/" {
		hu.Path = "/ws"
	} else if !strings.HasSuffix(hu.Path, "/ws") {
		hu.Path = strings.TrimRight(hu.Path, "/") + "/ws"
	}
	wsURL = hu.String()
	return wsURL, serverHTTP, nil
}

// WriteJSONWithTimeout sends a command using the provided WSClient with a
// timeout. It preserves the previous behaviour of returning a specific error
// if the send queue is full or there's no connection within the timeout.
func WriteJSONWithTimeout(ctx context.Context, wsClient *WSClient, cmd types.Command, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- wsClient.Send(cmd)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("send queue full or no connection")
	case <-ctx.Done():
		return ctx.Err()
	}
}
