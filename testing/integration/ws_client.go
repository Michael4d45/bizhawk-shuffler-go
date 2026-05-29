package integration

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michael4d45/bizshuffle/protocol"
)

// WSTestClient is a minimal WebSocket client for protocol integration tests.
type WSTestClient struct {
	conn    *websocket.Conn
	inbox   []protocol.Command
	wsURL   string
	mu      sync.Mutex
	writeMu sync.Mutex // gorilla/websocket allows one writer at a time
	// SaveUploadBase, when set, uploads a minimal save on request_save before acking.
	SaveUploadBase string
}

// NewWSTestClient builds a client for the server's /ws endpoint.
func NewWSTestClient(httpBase string) *WSTestClient {
	return &WSTestClient{wsURL: HTTPToWS(httpBase)}
}

// HTTPToWS converts an http:// base URL to ws://…/ws.
func HTTPToWS(httpURL string) string {
	u := strings.TrimSuffix(httpURL, "/")
	if strings.HasPrefix(u, "https://") {
		return "wss://" + strings.TrimPrefix(u, "https://") + "/ws"
	}
	return "ws://" + strings.TrimPrefix(u, "http://") + "/ws"
}

// Connect dials the WebSocket endpoint.
func (c *WSTestClient) Connect() error {
	conn, resp, err := websocket.DefaultDialer.Dial(c.wsURL, nil)
	if err != nil {
		return err
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	c.conn = conn
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var cmd protocol.Command
			if json.Unmarshal(data, &cmd) != nil {
				continue
			}
			if cmd.Cmd == protocol.CmdRequestSave && c.SaveUploadBase != "" {
				c.mu.Lock()
				c.inbox = append(c.inbox, cmd)
				c.mu.Unlock()
				go c.respondRequestSave(cmd)
				continue
			}
			if cmd.ID != "" && cmd.Cmd != protocol.CmdAck && cmd.Cmd != protocol.CmdNack {
				_ = c.Send(protocol.Command{Cmd: protocol.CmdAck, ID: cmd.ID})
			}
			c.mu.Lock()
			c.inbox = append(c.inbox, cmd)
			c.mu.Unlock()
		}
	}()
	return nil
}

// Send writes a command JSON frame.
func (c *WSTestClient) Send(cmd protocol.Command) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// Hello registers a player.
func (c *WSTestClient) Hello(name string, bizhawkReady bool) error {
	return c.Send(protocol.Command{
		Cmd: protocol.CmdHello,
		ID:  fmt.Sprintf("hello-%d", time.Now().UnixNano()),
		Payload: map[string]any{
			"name":          name,
			"bizhawk_ready": bizhawkReady,
		},
	})
}

// HelloAdmin registers an admin client.
func (c *WSTestClient) HelloAdmin(name string) error {
	return c.Send(protocol.Command{
		Cmd: protocol.CmdHelloAdmin,
		ID:  fmt.Sprintf("hello-admin-%d", time.Now().UnixNano()),
		Payload: map[string]any{
			"name": name,
		},
	})
}

// WaitFor returns the first inbox command matching predicate within timeout.
func (c *WSTestClient) WaitFor(match func(protocol.Command) bool, timeout time.Duration) (protocol.Command, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		for _, cmd := range c.inbox {
			if match(cmd) {
				c.mu.Unlock()
				return cmd, nil
			}
		}
		c.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
	return protocol.Command{}, fmt.Errorf("timeout waiting for command")
}

func (c *WSTestClient) respondRequestSave(cmd protocol.Command) {
	instanceID := ""
	if m, ok := cmd.Payload.(map[string]any); ok {
		if id, ok := m["instance_id"].(string); ok {
			instanceID = id
		}
	}
	if instanceID != "" && c.SaveUploadBase != "" {
		_ = UploadMinimalSave(c.SaveUploadBase, instanceID)
	}
	if cmd.ID != "" {
		_ = c.Send(protocol.Command{Cmd: protocol.CmdAck, ID: cmd.ID})
	}
}

// Close ends the connection.
func (c *WSTestClient) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}
