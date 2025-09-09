package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
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

// BizhawkIPC provides a small bridge to the Lua script listening on a TCP port
type BizhawkIPC struct {
	addr     string
	mu       sync.Mutex
	conn     net.Conn
	reader   *bufio.Reader
	pending  map[string]*pendingCmd
	incoming chan string
	closed   bool
	// ready indicates whether the Lua side has completed its HELLO handshake
	// and the IPC is considered ready to accept commands. Use the provided
	// accessor methods to read/update this flag.
	readyMu sync.Mutex
	ready   bool

	instanceID string
	game       string
	running    bool
}

// NewBizhawkIPC creates an instance targeting host:port
func NewBizhawkIPC(host string, port int) *BizhawkIPC {
	return &BizhawkIPC{
		addr:     fmt.Sprintf("%s:%d", host, port),
		pending:  make(map[string]*pendingCmd),
		incoming: make(chan string, 16),
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
	}()
	go func() {
		log.Printf("bizhawk ipc: starting resendLoop goroutine")
		b.resendLoop(ctx)
		log.Printf("bizhawk ipc: resendLoop goroutine exited")
	}()
	return nil
}

func (b *BizhawkIPC) connect() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		return nil
	}
	log.Printf("bizhawk ipc: attempting connect to %s; goroutines=%d pending=%d", b.addr, runtime.NumGoroutine(), len(b.pending))
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
	for id, pc := range b.pending {
		select {
		case pc.ch <- errors.New("ipc closed"):
		default:
		}
		delete(b.pending, id)
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
	id := strconv.FormatInt(time.Now().UnixNano(), 10)
	line := "CMD|" + id + "|" + strings.Join(parts, "|")

	pc := &pendingCmd{id: id, ch: make(chan error, 1), sentAt: time.Now(), attempts: 0, line: line}
	b.mu.Lock()
	b.pending[id] = pc
	b.mu.Unlock()

	// send immediately
	if err := b.sendLine(line); err != nil {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
		log.Printf("bizhawk ipc: SendCommand sendLine failed: %v", err)
		return err
	}
	pc.sentAt = time.Now()
	pc.attempts++

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-pc.ch:
		return err
	case <-time.After(5 * time.Second):
		return errors.New("timeout waiting for ACK")
	}
}

// readLoop reads incoming lines and dispatches them
func (b *BizhawkIPC) readLoop(ctx context.Context) {
	for {
		b.mu.Lock()
		r := b.reader
		b.mu.Unlock()
		if r == nil {
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
			r = b.reader
		}
		line, err := r.ReadString('\n')
		if err != nil {
			// connection closed; clear conn and retry
			log.Printf("bizhawk ipc: readLoop detected read error: %v; will clear conn and notify", err)
			b.mu.Lock()
			if b.conn != nil {
				_ = b.conn.Close()
			}
			b.conn = nil
			b.reader = nil

			// notify any pending commands that the IPC disconnected so callers
			// waiting for ACK/NACK don't block indefinitely.
			for id, pc := range b.pending {
				select {
				case pc.ch <- errors.New("ipc disconnected"):
				default:
				}
				delete(b.pending, id)
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
				log.Printf("bizhawk ipc: notifying MsgDisconnected; goroutines=%d pending=%d stack:\n%s", runtime.NumGoroutine(), len(b.pending), string(buf[:m]))
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
			if pc, ok := b.pending[id]; ok {
				pc.ch <- nil
				log.Printf("bizhawk ipc: ACK received for id=%s", id)
				delete(b.pending, id)
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
			if pc, ok := b.pending[id]; ok {
				pc.ch <- fmt.Errorf("nack: %s", reason)
				log.Printf("bizhawk ipc: NACK received for id=%s reason=%s", id, reason)
				delete(b.pending, id)
			}
			b.mu.Unlock()
		}
	case msgHELLO:
		// Lua said HELLO, we might want to send a SYNC later. Push incoming event.
		if !b.safeSend(line) {
			// dropped because incoming is full or closed - log at debug level
			log.Printf("bizhawk ipc: incoming channel full or closed, dropping HELLO")
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

// resendLoop retries pending commands periodically
func (b *BizhawkIPC) resendLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			b.mu.Lock()
			for id, pc := range b.pending {
				if pc.attempts >= 3 {
					pc.ch <- errors.New("max attempts reached")
					delete(b.pending, id)
					continue
				}
				if now.Sub(pc.sentAt) > 2*time.Second {
					// resend the stored original line
					if pc.line != "" {
						_ = b.sendLine(pc.line)
						pc.sentAt = now
						pc.attempts++
					} else {
						pc.ch <- errors.New("no original line to resend")
						delete(b.pending, id)
					}
				}
			}
			b.mu.Unlock()
		}
	}
}

// SendSync sends a SYNC command with game, state (running|stopped), and startAt epoch
func (b *BizhawkIPC) SendSync(ctx context.Context, game string, instanceID string, running bool) error {
	state := "stopped"
	if running {
		state = "running"
	}
	b.game = game
	b.instanceID = instanceID
	b.running = running
	return b.SendCommand(ctx, "SYNC", game, instanceID, state)
}

// Incoming returns the channel with raw lines from Lua for processing
func (b *BizhawkIPC) Incoming() <-chan string { return b.incoming }

func (b *BizhawkIPC) SendSave(ctx context.Context) error {
	return b.SendCommand(ctx, "SAVE")
}

// convenience helpers to match previous code
func (b *BizhawkIPC) SendSwap(ctx context.Context, game string, instanceID string) error {
	b.instanceID = instanceID
	return b.SendCommand(ctx, "SWAP", game, instanceID)
}

func (b *BizhawkIPC) SendStart(ctx context.Context, game string, instanceID string) error {
	b.instanceID = instanceID
	b.game = game
	b.running = true
	return b.SendCommand(ctx, "START", game, instanceID)
}

func (b *BizhawkIPC) SendPause(ctx context.Context) error {
	b.running = false
	return b.SendCommand(ctx, "PAUSE")
}

func (b *BizhawkIPC) SendResume(ctx context.Context) error {
	b.running = true
	return b.SendCommand(ctx, "RESUME")
}

func (b *BizhawkIPC) SendMessage(ctx context.Context, msg string) error {
	return b.SendCommand(ctx, "MSG", msg)
}

// SetReady sets the internal ready flag. Callers should use this to mark
// the IPC as ready/unready when a HELLO/SYNC handshake is observed or when
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
