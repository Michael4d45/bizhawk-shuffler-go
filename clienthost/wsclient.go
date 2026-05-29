package clienthost

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michael4d45/bizshuffle/obslog"
	"github.com/michael4d45/bizshuffle/protocol"
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
	sendCh chan protocol.Command
	// incoming commands from the server
	cmdCh chan protocol.Command

	wg sync.WaitGroup

	// active websocket connection (protected by connMu)
	connMu sync.Mutex
	conn   *websocket.Conn

	api  *API
	bipc *BizhawkIPC

	// controller is the current command handler
	controller *Controller

	// name is the player name for hello messages
	name string

	// helloAck signals when hello has been acknowledged by server
	helloAck chan struct{}
}

// NewWSClient creates a client for wsURL.
// The client does nothing until Start() is called.
func NewWSClient(wsURL string, api *API, bipc *BizhawkIPC) *WSClient {
	return &WSClient{
		wsURL:    wsURL,
		sendCh:   make(chan protocol.Command, 64),
		api:      api,
		bipc:     bipc,
		helloAck: make(chan struct{}),
	}
}

// GetConnectionStatus returns whether the client is connected to the server and whether BizHawk is ready.
func (w *WSClient) GetConnectionStatus() (connected, bizhawkReady bool) {
	w.connMu.Lock()
	connected = w.conn != nil
	w.connMu.Unlock()

	if w.bipc != nil {
		bizhawkReady = w.bipc.IsReady()
	}
	return connected, bizhawkReady
}

// GetController returns the active controller if connected.
func (w *WSClient) GetController() *Controller {
	return w.controller
}

// SendBizhawkReadinessUpdate sends an update to the server about BizHawk readiness status.
func (w *WSClient) SendBizhawkReadinessUpdate(ready bool) error {
	connected, _ := w.GetConnectionStatus()
	obslog.Event(obslog.WS, "bizhawk_ready_update", map[string]string{
		"ready":         fmt.Sprintf("%v", ready),
		"ws_connected":  fmt.Sprintf("%v", connected),
		"player":        w.name,
	})
	if !connected {
		// Hello on connect includes bizhawk_ready; avoid queueing status_update before WS is up.
		return nil
	}
	update := protocol.Command{
		Cmd: protocol.CmdStatusUpdate,
		Payload: map[string]any{
			"bizhawk_ready": ready,
		},
	}
	return w.Send(update)
}

// Start begins the connection and goroutines. It waits for hello acknowledgment before returning.
func (w *WSClient) Start(parent context.Context, cfg Config) {
	if w.ctx != nil {
		return // already started
	}

	w.name = cfg["name"]

	ctx, cancel := context.WithCancel(parent)
	w.ctx = ctx
	w.cancel = cancel

	// start connection manager (handles connect/reconnect)
	w.wg.Add(1)
	go w.run()

	// channel for incoming commands
	w.cmdCh = make(chan protocol.Command, 64)

	// start controller loop (handles incoming commands)
	sendFunc := func(cmd protocol.Command) error {
		return w.SendWithTimeout(cmd, 2*time.Second)
	}
	w.controller = NewControllerWithHelloAck(cfg, w.bipc, w.api, sendFunc, w.helloAck)
	go w.runController(ctx, w.controller)

	// wait for hello acknowledgment or context cancellation
	log.Printf("wsclient: waiting for hello acknowledgment from server...")
	select {
	case <-w.helloAck:
		log.Printf("wsclient: hello acknowledged, client startup complete")
	case <-ctx.Done():
		log.Printf("wsclient: context cancelled during hello wait")
	}
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

	// Reset context state so Start() can be called again
	w.ctx = nil
	w.cancel = nil
}

// Send enqueues a command for sending. Returns error if client is stopped.
func (w *WSClient) Send(cmd protocol.Command) error {
	if w.ctx == nil {
		return fmt.Errorf("wsclient stopped")
	}
	select {
	case w.sendCh <- cmd:
		return nil
	case <-w.ctx.Done():
		return fmt.Errorf("wsclient stopped")
	}
}

// SendWithTimeout tries to send a command, but fails if it takes too long.
func (w *WSClient) SendWithTimeout(cmd protocol.Command, timeout time.Duration) error {
	if w.ctx == nil {
		return fmt.Errorf("wsclient stopped")
	}
	done := make(chan error, 1)
	go func() { done <- w.Send(cmd) }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("send queue full or no connection")
	case <-w.ctx.Done():
		if w.ctx != nil {
			return w.ctx.Err()
		}
		return fmt.Errorf("wsclient stopped")
	}
}

// run manages the websocket connection.
// It reconnects automatically if the connection drops.
func (w *WSClient) run() {
	defer w.wg.Done()
	dialer := websocket.Dialer{
		NetDial:          (&net.Dialer{Timeout: 5 * time.Second}).Dial,
		HandshakeTimeout: 5 * time.Second,
	}

	for {
		// stop if context is canceled
		select {
		case <-w.ctx.Done():
			log.Printf("wsclient: run loop context done, exiting")
			return
		default:
		}

		// try to connect
		conn, resp, err := dialer.Dial(w.wsURL, nil)
		if err != nil {
			log.Printf("wsclient: dial error: %v; retrying in 2s", err)
			obslog.Event(obslog.WS, "dial_failed", map[string]string{
				"ws_url": w.wsURL,
				"error":  err.Error(),
			})
			select {
			case <-time.After(2 * time.Second):
				continue
			case <-w.ctx.Done():
				return
			}
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		log.Printf("wsclient: connected to %s", w.wsURL)
		obslog.Event(obslog.WS, "connected", map[string]string{"ws_url": w.wsURL})

		// record active connection
		w.connMu.Lock()
		w.conn = conn
		w.connMu.Unlock()

		// start writer goroutine
		writeDone := make(chan struct{})
		go w.writer(conn, writeDone)

		// send hello message with BizHawk readiness status
		bizhawkReady := false
		if w.bipc != nil {
			bizhawkReady = w.bipc.IsReady()
		}
		hello := protocol.Command{
			Cmd: protocol.CmdHello,
			Payload: map[string]any{
				"name":          w.name,
				"bizhawk_ready": bizhawkReady,
			},
		}
		if err := w.Send(hello); err != nil {
			log.Printf("wsclient: failed to send hello: %v", err)
			_ = conn.Close()
			continue
		}
		log.Printf("wsclient: sent hello as %s (bizhawk_ready: %v)", w.name, bizhawkReady)
		obslog.Event(obslog.WS, "hello_sent", map[string]string{
			"player":        w.name,
			"bizhawk_ready": fmt.Sprintf("%v", bizhawkReady),
		})

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
		var cmd protocol.Command
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
			if cmd.Cmd == protocol.CmdSwap {
				obslog.Event(obslog.Swap, "received", map[string]string{
					"payload": fmt.Sprintf("%v", cmd.Payload),
				})
			}
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
func (w *WSClient) enqueueCommand(cmd protocol.Command) {
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
