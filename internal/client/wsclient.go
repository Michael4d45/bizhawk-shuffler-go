package client

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michael4d45/bizshuffle/internal/types"
)

// WSClient is a simple reconnecting websocket client. It runs an internal
// writer goroutine that takes Commands from a buffered channel and writes
// them to the server. Incoming messages are decoded and passed to the
// registered handler function.
type WSClient struct {
	wsURL  string
	ctx    context.Context
	cancel func()

	sendCh chan types.Command

	handlerMu sync.RWMutex
	handler   func(types.Command)

	wg sync.WaitGroup
	// track the active websocket connection so Stop() can close it and
	// unblock any blocking Read/Write calls.
	connMu sync.Mutex
	conn   *websocket.Conn
}

// NewWSClient creates a client for wsURL. The returned client is not started
// until Start is called.
func NewWSClient(wsURL string) *WSClient {
	return &WSClient{wsURL: wsURL, sendCh: make(chan types.Command, 64)}
}

// Start begins the connection and goroutines. It returns immediately.
func (w *WSClient) Start(parent context.Context) {
	if w.ctx != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.ctx = ctx
	w.cancel = cancel
	w.wg.Add(1)
	go w.run()
}

// Stop signals the client to stop and waits for goroutines to exit.
func (w *WSClient) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	// close the active connection to ensure the reader goroutine unblocks
	w.connMu.Lock()
	if w.conn != nil {
		_ = w.conn.Close()
		w.conn = nil
	}
	w.connMu.Unlock()
	w.wg.Wait()
}

// Send enqueues a command for sending. Returns error if client is stopped.
func (w *WSClient) Send(cmd types.Command) error {
	select {
	case w.sendCh <- cmd:
		return nil
	case <-w.ctx.Done():
		return fmt.Errorf("wsclient stopped")
	}
}

// RegisterHandler sets a function to receive incoming messages.
func (w *WSClient) RegisterHandler(h func(types.Command)) {
	w.handlerMu.Lock()
	w.handler = h
	w.handlerMu.Unlock()
}

func (w *WSClient) run() {
	defer w.wg.Done()
	dialer := websocket.Dialer{}
	for {
		select {
		case <-w.ctx.Done():
			log.Printf("wsclient: run loop context done, exiting")
			return
		default:
		}
		conn, _, err := dialer.Dial(w.wsURL, nil)
		if err != nil {
			log.Printf("wsclient: ws dial error: %v; retrying in 2s", err)
			select {
			case <-time.After(2 * time.Second):
				continue
			case <-w.ctx.Done():
				return
			}
		}
		log.Printf("wsclient: websocket connected to server %s", w.wsURL)

		// record active connection so Stop() can close it
		w.connMu.Lock()
		w.conn = conn
		w.connMu.Unlock()

		// writer
		writeDone := make(chan struct{})
		go func() {
			defer func() {
				close(writeDone)
				log.Printf("wsclient: writer goroutine exiting")
			}()
			for {
				select {
				case cmd := <-w.sendCh:
					if err := conn.WriteJSON(cmd); err != nil {
						log.Printf("wsclient: ws write error: %v", err)
						return
					}
					log.Printf("wsclient: wrote cmd: %v", cmd)
				case <-w.ctx.Done():
					log.Printf("wsclient: writer received ctx.Done()")
					return
				}
			}
		}()

		// reader
		for {
			var cmd types.Command
			if err := conn.ReadJSON(&cmd); err != nil {
				log.Printf("wsclient: ws read error: %v", err)
				break
			}
			w.handlerMu.RLock()
			h := w.handler
			w.handlerMu.RUnlock()
			if h != nil {
				// protect the handler invocation so a panic inside handler doesn't
				// crash the wsclient goroutine; log panic if it happens.
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("wsclient: handler panic: %v", r)
						}
					}()
					h(cmd)
				}()
			} else {
				log.Printf("wsclient: no handler registered for incoming cmd: %v", cmd)
			}
		}

		// close connection and wait for writer to finish
		_ = conn.Close()
		// clear recorded connection
		w.connMu.Lock()
		if w.conn == conn {
			w.conn = nil
		}
		w.connMu.Unlock()
		select {
		case <-writeDone:
		case <-time.After(1 * time.Second):
		}
		// loop and reconnect
	}
}
