package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michael4d45/bizshuffle/internal/types"
)

// wsClient represents a connected websocket client and its outbound send queue
type wsClient struct {
	conn   *websocket.Conn
	sendCh chan types.Command
}

// handleWS upgrades to websocket and manages client lifecycle.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	client := &wsClient{conn: c, sendCh: make(chan types.Command, 8)}
	s.mu.Lock()
	s.conns[c] = client
	s.mu.Unlock()

	c.SetReadLimit(1024 * 16)
	c.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.SetPongHandler(func(string) error { c.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })

	var writeWG sync.WaitGroup
	writeWG.Add(1)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer func() { ticker.Stop(); writeWG.Done() }()
		for {
			select {
			case cmd, ok := <-client.sendCh:
				c.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if !ok {
					c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					return
				}
				if err := c.WriteJSON(cmd); err != nil {
					log.Printf("write json err: %v", err)
					return
				}
			case <-ticker.C:
				c.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	defer func() {
		close(client.sendCh)
		writeWG.Wait()
		s.mu.Lock()
		if cl, ok := s.conns[c]; ok {
			name := ""
			for n, pc := range s.players {
				if pc == cl {
					name = n
					break
				}
			}
			if name != "" {
				pl := s.state.Players[name]
				pl.Connected = false
				s.state.Players[name] = pl
				delete(s.players, name)
			}
			delete(s.conns, c)
		}
		s.mu.Unlock()
		s.saveState()
		c.Close()
	}()

	for {
		var cmd types.Command
		if err := c.ReadJSON(&cmd); err != nil {
			log.Printf("read: %v", err)
			break
		}
		log.Printf("received cmd from client: %s id=%s", cmd.Cmd, cmd.ID)

		if cmd.Cmd == types.CmdAck || cmd.Cmd == types.CmdNack {
			s.mu.Lock()
			ch, ok := s.pending[cmd.ID]
			if ok {
				if cmd.Cmd == types.CmdAck {
					select {
					case ch <- "ack":
					default:
					}
				} else {
					reason := "nack"
					if cmd.Payload != nil {
						if b, err := json.Marshal(cmd.Payload); err == nil {
							reason = "nack|" + string(b)
						}
					}
					log.Printf("received nack id=%s payload=%+v", cmd.ID, cmd.Payload)
					select {
					case ch <- reason:
					default:
					}
				}
				close(ch)
				delete(s.pending, cmd.ID)
			}
			s.mu.Unlock()
			continue
		}
		if cmd.Cmd == types.CmdStatus || cmd.Cmd == types.CmdStateUpdate {
			s.mu.Lock()
			var pname string
			for n, pc := range s.players {
				if pc == client {
					pname = n
					break
				}
			}
			if pname != "" {
				if pl, ok := cmd.Payload.(map[string]any); ok {
					if st, ok := pl["status"].(string); ok {
						s.ephemeral[pname] = st
					}
				}
			}
			s.mu.Unlock()
			continue
		}
		if cmd.Cmd == types.CmdGamesUpdateAck {
			s.mu.Lock()
			var pname string
			for n, pc := range s.players {
				if pc == client {
					pname = n
					break
				}
			}
			if pname != "" {
				if pl, ok := cmd.Payload.(map[string]any); ok {
					if hf, ok := pl["has_files"].(bool); ok {
						p := s.state.Players[pname]
						p.HasFiles = hf
						s.state.Players[pname] = p
						s.state.UpdatedAt = time.Now()
					}
				}
			}
			updatedAt := s.state.UpdatedAt
			s.mu.Unlock()
			s.saveState()
			s.broadcast(types.Command{Cmd: types.CmdStateUpdate, Payload: map[string]any{"updated_at": updatedAt}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
			continue
		}
		if cmd.Cmd == types.CmdHello {
			if pl, ok := cmd.Payload.(map[string]any); ok {
				name := "player"
				if v, ok := pl["name"].(string); ok {
					name = v
				}
				s.mu.Lock()
				s.state.Players[name] = types.Player{Name: name, Connected: true}
				s.conns[c] = client
				s.players[name] = client
				games := append([]string{}, s.state.Games...)
				mainGames := append([]types.GameEntry{}, s.state.MainGames...)
				s.mu.Unlock()
				s.saveState()
				payload := map[string]any{"games": games, "main_games": mainGames}
				select {
				case client.sendCh <- types.Command{Cmd: types.CmdGamesUpdate, Payload: payload, ID: fmt.Sprintf("%d", time.Now().UnixNano())}:
				default:
				}
			}
			continue
		}
		log.Printf("client message: %+v", cmd)
	}
}

// sendToPlayer enqueues a command to a player's websocket send queue.
func (s *Server) sendToPlayer(player string, cmd types.Command) error {
	s.mu.Lock()
	client, ok := s.players[player]
	s.mu.Unlock()
	if !ok || client == nil {
		return fmt.Errorf("no connection for player %s", player)
	}
	select {
	case client.sendCh <- cmd:
		return nil
	default:
		return fmt.Errorf("send queue full for player %s", player)
	}
}

// waitForResult waits for result channel with timeout.
func (s *Server) waitForResult(cmdID string, timeout time.Duration) (string, error) {
	ch := make(chan string, 1)
	s.mu.Lock()
	s.pending[cmdID] = ch
	s.mu.Unlock()
	select {
	case res := <-ch:
		return res, nil
	case <-time.After(timeout):
		s.mu.Lock()
		delete(s.pending, cmdID)
		s.mu.Unlock()
		return "", fmt.Errorf("timeout waiting for result %s", cmdID)
	}
}

// sendAndWait convenience wrapper that registers pending and waits for ack/nack.
func (s *Server) sendAndWait(player string, cmd types.Command, timeout time.Duration) (string, error) {
	ch := make(chan string, 1)
	s.mu.Lock()
	s.pending[cmd.ID] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.pending, cmd.ID); s.mu.Unlock() }()
	if err := s.sendToPlayer(player, cmd); err != nil {
		return "", err
	}
	select {
	case res := <-ch:
		return res, nil
	case <-time.After(timeout):
		return "", ErrTimeout
	}
}
