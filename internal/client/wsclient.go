package client

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michael4d45/bizshuffle/internal/types"
)

// WSClient is a reconnecting websocket client.
// It has three main jobs:
//  1. Manage the websocket connection (connect, reconnect on errors).
//  2. Send outgoing commands (via a writer goroutine).
//  3. Receive incoming commands and pass them to a controller.
type WSClient struct {
	wsURL  string
	ctx    context.Context
	cancel func()

	// outgoing commands to the server
	sendCh chan types.Command
	// incoming commands from the server
	cmdCh chan types.Command

	wg sync.WaitGroup

	// active websocket connection (protected by connMu)
	connMu sync.Mutex
	conn   *websocket.Conn

	api  *API
	bipc *BizhawkIPC
}

// NewWSClient creates a client for wsURL.
// The client does nothing until Start() is called.
func NewWSClient(wsURL string, api *API, bipc *BizhawkIPC) *WSClient {
	return &WSClient{
		wsURL:  wsURL,
		sendCh: make(chan types.Command, 64),
		api:    api,
		bipc:   bipc,
	}
}

// Start begins the connection and goroutines. It returns immediately.
func (w *WSClient) Start(parent context.Context, cfg Config) {
	if w.ctx != nil {
		return // already started
	}

	ctx, cancel := context.WithCancel(parent)
	w.ctx = ctx
	w.cancel = cancel

	// start connection manager (handles connect/reconnect)
	w.wg.Add(1)
	go w.run()

	// channel for incoming commands
	w.cmdCh = make(chan types.Command, 64)

	// send initial hello message
	hello := types.Command{
		Cmd:     types.CmdHello,
		Payload: map[string]string{"name": cfg["name"]},
	}
	sendFunc := func(cmd types.Command) error {
		return w.SendWithTimeout(cmd, 2*time.Second)
	}
	_ = sendFunc(hello)
	// start controller loop (handles incoming commands)
	controller := NewController(cfg, w.bipc, w.api, sendFunc)
	go w.runController(ctx, controller)
}

// Stop signals the client to stop and waits for goroutines to exit.
func (w *WSClient) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	// close the active connection to unblock reader/writer
	w.connMu.Lock()
	if w.conn != nil {
		_ = w.conn.Close()
		w.conn = nil
	}
	w.connMu.Unlock()
	w.wg.Wait()

	// Close cmdCh to unblock runController if needed
	close(w.cmdCh)
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

// SendWithTimeout tries to send a command, but fails if it takes too long.
func (w *WSClient) SendWithTimeout(cmd types.Command, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- w.Send(cmd) }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("send queue full or no connection")
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

// run manages the websocket connection.
// It reconnects automatically if the connection drops.
func (w *WSClient) run() {
	defer w.wg.Done()
	dialer := websocket.Dialer{}

	for {
		// stop if context is canceled
		select {
		case <-w.ctx.Done():
			log.Printf("wsclient: run loop context done, exiting")
			return
		default:
		}

		// try to connect
		conn, _, err := dialer.Dial(w.wsURL, nil)
		if err != nil {
			log.Printf("wsclient: dial error: %v; retrying in 2s", err)
			select {
			case <-time.After(2 * time.Second):
				continue
			case <-w.ctx.Done():
				return
			}
		}
		log.Printf("wsclient: connected to %s", w.wsURL)

		// record active connection
		w.connMu.Lock()
		w.conn = conn
		w.connMu.Unlock()

		// start writer goroutine
		writeDone := make(chan struct{})
		go w.writer(conn, writeDone)

		// run reader loop (blocking)
		w.reader(conn)

		// cleanup after disconnect
		_ = conn.Close()
		w.connMu.Lock()
		if w.conn == conn {
			w.conn = nil
		}
		w.connMu.Unlock()

		// wait for writer to finish
		select {
		case <-writeDone:
		case <-time.After(1 * time.Second):
		}

		// loop and reconnect
	}
}

// writer sends commands from sendCh to the websocket.
func (w *WSClient) writer(conn *websocket.Conn, done chan struct{}) {
	defer func() {
		close(done)
		log.Printf("wsclient: writer exiting")
	}()
	for {
		select {
		case cmd := <-w.sendCh:
			if err := conn.WriteJSON(cmd); err != nil {
				log.Printf("wsclient: write error: %v", err)
				return
			}
			log.Printf("wsclient: sent cmd: %v", cmd)
		case <-w.ctx.Done():
			log.Printf("wsclient: writer context cancelled, exiting")
			return
		}
	}
}

// reader receives commands from the websocket and enqueues them.
func (w *WSClient) reader(conn *websocket.Conn) {
	defer log.Printf("wsclient: reader exiting")

	// Create a channel to signal when we should stop
	done := make(chan struct{})
	defer close(done)

	// Goroutine to handle context cancellation
	go func() {
		select {
		case <-w.ctx.Done():
			// Close the connection to unblock ReadJSON
			_ = conn.Close()
		case <-done:
			// Reader is exiting normally
		}
	}()

	for {
		var cmd types.Command
		if err := conn.ReadJSON(&cmd); err != nil {
			log.Printf("wsclient: read error: %v", err)
			return
		}
		// protect against panics in enqueue
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("wsclient: enqueue panic: %v", r)
				}
			}()
			w.enqueueCommand(cmd)
		}()
	}
}

// runController handles incoming commands from cmdCh.
func (w *WSClient) runController(ctx context.Context, controller *Controller) {
	defer log.Printf("controller loop exiting")
	for {
		select {
		case <-ctx.Done():
			return
		case cmd, ok := <-w.cmdCh:
			if !ok {
				return
			}
			log.Printf("server->client cmd: %s", cmd.Cmd)
			log.Printf("cmd payload: %+v", cmd.Payload)
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("controller.Handle panic: %v", r)
					}
				}()
				controller.Handle(ctx, cmd)
			}()
		}
	}
}

// enqueueCommand puts a command into cmdCh, or drops it if full.
func (w *WSClient) enqueueCommand(cmd types.Command) {
	select {
	case w.cmdCh <- cmd:
	default:
		ir := false
		if w.bipc != nil {
			ir = w.bipc.IsReady()
		}
		log.Printf("incoming command dropped: %v; goroutines=%d ipcReady=%v",
			cmd.Cmd, runtime.NumGoroutine(), ir)
	}
}
