package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Message types from Lua/BizHawk side
const (
	msgACK   = "ACK"
	msgNACK  = "NACK"
	msgHELLO = "HELLO"
	msgCMD   = "CMD"
	msgPING  = "PING"
	// sentinel used to notify consumers that the IPC connection was lost
	// exported so callers can react when the Lua side disconnects.
	MsgDisconnected = "__BIZHAWK_IPC_DISCONNECTED__"
)

// Pending command waiting for ack
type pendingCmd struct {
	id       string
	ch       chan error
	sentAt   time.Time
	attempts int
	line     string
}

// Queued command to be sent sequentially
type queuedCmd struct {
	parts []string
	ch    chan error
}

// BizhawkIPC provides a small bridge to the Lua script listening on a TCP port
type BizhawkIPC struct {
	addr     string
	mu       sync.Mutex
	conn     net.Conn
	reader   *bufio.Reader
	pending  *pendingCmd
	incoming chan string
	closed   bool
	// ready indicates whether the Lua side has completed its HELLO handshake
	// and the IPC is considered ready to accept commands. Use the provided
	// accessor methods to read/update this flag.
	readyMu sync.Mutex
	ready   bool

	commandQueue chan *queuedCmd

	instanceID string
	game       string
	running    bool
}

// NewBizhawkIPC creates an instance targeting host:port
func NewBizhawkIPC() *BizhawkIPC {
	port := 55355 // default port
	// check if port is already used; if so, increment until we find a free one
	for {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			port++
			continue
		}
		_ = ln.Close()
		break
	}
	// save port to lua_server_port.txt for Lua script to read
	if err := writePortFile(port); err != nil {
		log.Printf("bizhawk ipc: failed to write port file: %v", err)
	}
	return &BizhawkIPC{
		addr:         fmt.Sprintf("%s:%d", "127.0.0.1", port),
		pending:      nil,
		incoming:     make(chan string, 16),
		commandQueue: make(chan *queuedCmd, 16),
	}
}

// Start connects and starts background readers and resender
func (b *BizhawkIPC) Start(ctx context.Context) error {
	if err := b.connect(); err != nil {
		// initial connect failed; the readLoop has reconnect logic so
		// start background goroutines anyway and let them retry.
		// Return nil so callers don't assume a persistent failure.
		// Log the error so it's visible.
		log.Printf("bizhawk ipc: initial connect failed, will retry: %v", err)
	}
	go func() {
		log.Printf("bizhawk ipc: starting readLoop goroutine")
		b.readLoop(ctx)
		log.Printf("bizhawk ipc: readLoop goroutine exited")
		// delete lua_server_port.txt
		_ = os.Remove("lua_server_port.txt")
	}()
	go func() {
		log.Printf("bizhawk ipc: starting commandProcessor goroutine")
		b.commandProcessor(ctx)
		log.Printf("bizhawk ipc: commandProcessor goroutine exited")
	}()
	return nil
}

func (b *BizhawkIPC) connect() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		return nil
	}
	log.Printf("bizhawk ipc: attempting connect to %s; goroutines=%d pending=%d", b.addr, runtime.NumGoroutine(), func() int {
		if b.pending != nil {
			return 1
		} else {
			return 0
		}
	}())
	c, err := net.DialTimeout("tcp", b.addr, 2*time.Second)
	if err != nil {
		log.Printf("bizhawk ipc: connect error: %v", err)
		return err
	}
	b.conn = c
	b.reader = bufio.NewReader(c)
	return nil
}

func (b *BizhawkIPC) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	if b.conn != nil {
		_ = b.conn.Close()
		b.conn = nil
	}

	// notify any pending commands that the IPC is closing so callers
	// waiting for ACK/NACK don't block indefinitely.
	if b.pending != nil {
		select {
		case b.pending.ch <- errors.New("ipc closed"):
		default:
		}
		b.pending = nil
	}

	// closing incoming so consumers will see range() end
	close(b.incoming)
	log.Printf("bizhawk ipc: closed and incoming channel closed")
	return nil
}

func (b *BizhawkIPC) sendLine(line string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		return errors.New("not connected")
	}
	n, err := b.conn.Write([]byte(line + "\n"))
	if err != nil {
		log.Printf("bizhawk ipc: sendLine error writing %d bytes: %v", n, err)
		return err
	}
	log.Printf("bizhawk ipc: sendLine wrote %d bytes: %q", n, line)
	return nil
}

// SendCommand sends a command and waits for ACK or NACK or timeout
func (b *BizhawkIPC) SendCommand(ctx context.Context, parts ...string) error {
	qc := &queuedCmd{parts: parts, ch: make(chan error, 1)}
	select {
	case b.commandQueue <- qc:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-qc.ch:
		return err
	}
}

// readLoop reads incoming lines and dispatches them
func (b *BizhawkIPC) readLoop(ctx context.Context) {
	for {
		// Check context first
		select {
		case <-ctx.Done():
			log.Printf("bizhawk ipc: readLoop context cancelled, exiting")
			return
		default:
		}

		b.mu.Lock()
		r := b.reader
		conn := b.conn
		b.mu.Unlock()

		if r == nil || conn == nil {
			// try reconnect
			if err := b.connect(); err != nil {
				log.Printf("bizhawk ipc: connect failed in readLoop: %v", err)
				select {
				case <-ctx.Done():
					log.Printf("bizhawk ipc: readLoop context done while reconnecting")
					return
				case <-time.After(1 * time.Second):
					continue
				}
			}
			b.mu.Lock()
			r = b.reader
			conn = b.conn
			b.mu.Unlock()
		}

		// Set a read deadline to make ReadString cancellable
		if conn != nil {
			_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		}

		line, err := r.ReadString('\n')
		if err != nil {
			// Check if this is a timeout (which is expected for context checking)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// This is just our periodic timeout, continue the loop
				continue
			}

			// connection closed or real error; clear conn and retry
			log.Printf("bizhawk ipc: readLoop detected read error: %v; will clear conn and notify", err)
			b.mu.Lock()
			if b.conn != nil {
				_ = b.conn.Close()
			}
			b.conn = nil
			b.reader = nil

			// notify any pending commands that the IPC disconnected so callers
			// waiting for ACK/NACK don't block indefinitely.
			if b.pending != nil {
				select {
				case b.pending.ch <- errors.New("ipc disconnected"):
				default:
				}
				b.pending = nil
			}

			b.mu.Unlock()
			// notify listeners that the IPC connection was lost so callers can react
			b.mu.Lock()
			closed := b.closed
			b.mu.Unlock()
			if !closed {
				// dump brief stack and pending info to help debugging who is listening
				buf := make([]byte, 1<<12)
				m := runtime.Stack(buf, false)
				log.Printf("bizhawk ipc: notifying MsgDisconnected; goroutines=%d pending=%d stack:\n%s", runtime.NumGoroutine(), func() int {
					if b.pending != nil {
						return 1
					} else {
						return 0
					}
				}(), string(buf[:m]))
				if b.safeSend(MsgDisconnected) {
					log.Printf("bizhawk ipc: signaled MsgDisconnected to incoming consumers")
				} else {
					log.Printf("bizhawk ipc: incoming channel full or closed, dropping MsgDisconnected")
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
				continue
			}
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// dispatch
		log.Printf("bizhawk ipc: received line: %q", line)
		b.handleLine(line)
	}
}

func (b *BizhawkIPC) handleLine(line string) {
	parts := strings.Split(line, "|")
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case msgACK:
		if len(parts) >= 2 {
			id := parts[1]
			b.mu.Lock()
			if b.pending != nil && b.pending.id == id {
				b.pending.ch <- nil
				log.Printf("bizhawk ipc: ACK received for id=%s", id)
				b.pending = nil
			} else {
				log.Printf("bizhawk ipc: ACK received for id=%s but not in pending", id)
			}
			b.mu.Unlock()
		}
	case msgNACK:
		if len(parts) >= 2 {
			id := parts[1]
			reason := ""
			if len(parts) >= 3 {
				reason = strings.Join(parts[2:], "|")
			}
			b.mu.Lock()
			if b.pending != nil && b.pending.id == id {
				b.pending.ch <- fmt.Errorf("nack: %s", reason)
				log.Printf("bizhawk ipc: NACK received for id=%s reason=%s", id, reason)
				b.pending = nil
			}
			b.mu.Unlock()
		}
	case msgPING:
		// reply PONG
		if len(parts) >= 2 {
			ts := parts[1]
			if err := b.sendLine("PONG|" + ts); err != nil {
				log.Printf("bizhawk ipc: failed to send PONG: %v", err)
			} else {
				log.Printf("bizhawk ipc: replied with PONG %s", ts)
			}
		}
	default:
		// forward other messages to incoming channel
		if b.safeSend(line) {
			log.Printf("bizhawk ipc: forwarded message to incoming: %q", line)
		} else {
			log.Printf("bizhawk ipc: incoming channel full or closed, dropping message: %q", line)
		}
	}
}

// safeSend attempts to send a string to the incoming channel while holding
// the mutex to ensure we don't send on a closed channel. It performs a
// non-blocking send and returns true if the value was queued.
func (b *BizhawkIPC) safeSend(s string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}
	select {
	case b.incoming <- s:
		return true
	default:
		return false
	}
}

// commandProcessor processes queued commands sequentially
func (b *BizhawkIPC) commandProcessor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Printf("bizhawk ipc: commandProcessor context cancelled, exiting")
			return
		case qc := <-b.commandQueue:
			b.processCommand(ctx, qc)
		}
	}
}

// processCommand sends a command and waits for response
func (b *BizhawkIPC) processCommand(ctx context.Context, qc *queuedCmd) {
	id := strconv.FormatInt(time.Now().UnixNano(), 10)
	line := "CMD|" + id + "|" + strings.Join(qc.parts, "|")

	pc := &pendingCmd{id: id, ch: make(chan error, 1), sentAt: time.Now(), attempts: 1, line: line}
	b.mu.Lock()
	b.pending = pc
	b.mu.Unlock()

	if err := b.sendLine(line); err != nil {
		b.mu.Lock()
		b.pending = nil
		b.mu.Unlock()
		qc.ch <- err
		return
	}

	select {
	case <-ctx.Done():
		qc.ch <- ctx.Err()
	case err := <-pc.ch:
		qc.ch <- err
	case <-time.After(10 * time.Second):
		b.mu.Lock()
		if b.pending == pc {
			b.pending = nil
		}
		b.mu.Unlock()
		qc.ch <- fmt.Errorf("timeout waiting for ACK: %s", line)
	}
}

// Incoming returns the channel with raw lines from Lua for processing
func (b *BizhawkIPC) Incoming() <-chan string { return b.incoming }

func (b *BizhawkIPC) SendSave(ctx context.Context) error {
	return b.SendCommand(ctx, "SAVE")
}

// convenience helpers to match previous code
func (b *BizhawkIPC) SendSwap(ctx context.Context, game string, instanceID string) error {
	if err := b.SendSave(ctx); err != nil {
		log.Printf("ipc handler: SendSave failed before SWAP: %v", err)
		return err
	}
	b.instanceID = instanceID
	b.game = game
	return b.SendCommand(ctx, "SWAP", game, instanceID)
}

func (b *BizhawkIPC) SendPause(ctx context.Context) error {
	b.running = false
	return b.SendCommand(ctx, "PAUSE")
}

func (b *BizhawkIPC) SendResume(ctx context.Context) error {
	b.running = true
	return b.SendCommand(ctx, "RESUME")
}

func (b *BizhawkIPC) SendRestart(ctx context.Context) error {
	return b.SendCommand(ctx, "LOAD", b.game, b.instanceID)
}

func (b *BizhawkIPC) SendMessage(ctx context.Context, msg string) error {
	return b.SendCommand(ctx, "MSG", msg, "3.0", "10", "10", "12", "#FFFFFF", "#000000")
}

func (b *BizhawkIPC) SendStyledMessage(ctx context.Context, msg string, duration float64, x, y, fontsize int, fg, bg string) error {
	return b.SendCommand(ctx, "MSG", msg, fmt.Sprintf("%.1f", duration), strconv.Itoa(x), strconv.Itoa(y), strconv.Itoa(fontsize), fg, bg)
}

// SetReady sets the internal ready flag. Callers should use this to mark
// the IPC as ready/unready when a HELLO handshake is observed or when
// the connection is lost.
func (b *BizhawkIPC) SetReady(v bool) {
	b.readyMu.Lock()
	b.ready = v
	b.readyMu.Unlock()
}

// IsReady returns the current ready flag.
func (b *BizhawkIPC) IsReady() bool {
	b.readyMu.Lock()
	v := b.ready
	b.readyMu.Unlock()
	return v
}

// writePortFile writes the selected TCP port number to a file named
// "lua_server_port.txt" in the current working directory so the Lua script
// launched inside BizHawk can read which port to connect back to.
func writePortFile(port int) error {
	fname := "lua_server_port.txt"
	// ensure parent directory exists (for completeness if fname contained a path)
	// but since fname is a simple filename this will be a no-op. Keep for parity
	// with other write helpers in the project.
	// Write the port as plain text with a trailing newline to make it easy to read.
	data := fmt.Appendf(nil, "%d\n", port)
	dir := "."
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(fname, data, 0644)
}
