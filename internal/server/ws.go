package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
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

// findPlayerNameForClient returns the player name associated with the given wsClient or
// empty string if none. Caller must hold s.mu if concurrent access is possible.
func (s *Server) findPlayerNameForClient(client *wsClient) string {
	for n, pc := range s.players {
		if pc == client {
			return n
		}
	}
	return ""
}

// handleWS upgrades to websocket and manages client lifecycle.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	client := &wsClient{conn: c, sendCh: make(chan types.Command, 8)}
	s.withLock(func() {
		s.conns[c] = client
	})

	c.SetReadLimit(1024 * 16)
	if err := c.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		log.Printf("SetReadDeadline error: %v", err)
	}
	// Pong handler updated to compute RTT when we sent a timestamp in the ping payload.
	c.SetPongHandler(func(appData string) error {
		if err := c.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			log.Printf("SetReadDeadline error: %v", err)
		}
		// parse timestamp from pong appData (sent as unix nanoseconds string)
		if appData == "" {
			return nil
		}
		if ts, err := strconv.ParseInt(appData, 10, 64); err == nil {
			sent := time.Unix(0, ts)
			rtt := time.Since(sent)
			// Attempt to find player name for this client and store ping in server state
			pname := ""
			// findPlayerNameForClient requires s.mu for safe access
			s.withRLock(func() {
				pname = s.findPlayerNameForClient(client)
			})
			if pname != "" {
				_ = s.UpdateStateAndPersist(func(st *types.ServerState) {
					pl := st.Players[pname]
					pl.PingMs = int(rtt.Milliseconds())
					st.Players[pname] = pl
				})
			}
		}
		return nil
	})

	var writeWG sync.WaitGroup
	writeWG.Add(1)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer func() { ticker.Stop(); writeWG.Done() }()
		for {
			select {
			case cmd, ok := <-client.sendCh:
				if err := c.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
					log.Printf("SetWriteDeadline error: %v", err)
				}
				if !ok {
					if err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
						log.Printf("write close msg err: %v", err)
					}
					return
				}
				// Special-case ping command to send a control ping frame with payload so client can pong with timestamp.
				if cmd.Cmd == types.CmdPing {
					// payload should be a string timestamp in nanoseconds if provided; otherwise use now.
					payload := fmt.Sprintf("%d", time.Now().UnixNano())
					if p, ok := cmd.Payload.(string); ok && p != "" {
						payload = p
					}
					if err := c.WriteMessage(websocket.PingMessage, []byte(payload)); err != nil {
						log.Printf("write ping err: %v", err)
						return
					}
				} else {
					if err := c.WriteJSON(cmd); err != nil {
						log.Printf("write json err: %v", err)
						return
					}
				}
			case <-ticker.C:
				if err := c.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
					log.Printf("SetWriteDeadline error: %v", err)
				}
				// include timestamp payload (unix nano) so we can measure RTT on Pong
				payload := fmt.Sprintf("%d", time.Now().UnixNano())
				if err := c.WriteMessage(websocket.PingMessage, []byte(payload)); err != nil {
					log.Printf("write ping err: %v", err)
					return
				}
			}
		}
	}()

	defer func() {
		close(client.sendCh)
		writeWG.Wait()
		// remove connection and mark player disconnected under lock
		_ = s.UpdateStateAndPersist(func(st *types.ServerState) {
			if cl, ok := s.conns[c]; ok {
				name := s.findPlayerNameForClient(cl)
				if name != "" {
					pl := st.Players[name]
					pl.Connected = false
					st.Players[name] = pl
					delete(s.players, name)
				}
				delete(s.conns, c)
			}
		})
		if err := c.Close(); err != nil {
			log.Printf("websocket close error: %v", err)
		}
	}()

	for {
		var cmd types.Command
		if err := c.ReadJSON(&cmd); err != nil {
			log.Printf("read: %v", err)
			break
		}
		log.Printf("received cmd from client: %s id=%s", cmd.Cmd, cmd.ID)

		if cmd.Cmd == types.CmdAck || cmd.Cmd == types.CmdNack {
			var ch chan string
			var ok bool
			s.withRLock(func() {
				ch, ok = s.pending[cmd.ID]
			})
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
				// remove pending entry under write lock
				s.withLock(func() {
					delete(s.pending, cmd.ID)
				})
			}
			continue
		}
		if cmd.Cmd == types.CmdGamesUpdateAck {
			// determine player name and update under locks
			pname := ""
			s.withRLock(func() {
				pname = s.findPlayerNameForClient(client)
			})
			if pname != "" {
				if pl, ok := cmd.Payload.(map[string]any); ok {
					if hf, ok := pl["has_files"].(bool); ok {
						_ = s.UpdateStateAndPersist(func(st *types.ServerState) {
							p := st.Players[pname]
							p.HasFiles = hf
							st.Players[pname] = p
						})
						continue
					}
				}
			}
			continue
		}
		if cmd.Cmd == types.CmdHello {
			if pl, ok := cmd.Payload.(map[string]any); ok {
				name := ""
				if v, ok := pl["name"].(string); ok {
					name = v
				}
				if name == "" {
					log.Printf("CmdHello missing name in payload")
					continue
				}
				player := s.currentPlayer(name)
				player.Connected = true

				var instances []types.GameSwapInstance
				var mainGames []types.GameEntry
				var games []string
				_ = s.UpdateStateAndPersist(func(st *types.ServerState) {
					st.Players[name] = player
					s.conns[c] = client
					s.players[name] = client
					instances = append([]types.GameSwapInstance{}, st.GameSwapInstances...)
					mainGames = append([]types.GameEntry{}, st.MainGames...)
					games = append([]string{}, st.Games...)
				})

				payload := map[string]any{
					"game_instances": instances,
					"main_games":     mainGames,
					"games":          games,
				}
				select {
				case client.sendCh <- types.Command{
					Cmd:     types.CmdGamesUpdate,
					Payload: payload,
					ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
				}:

				default:
					fmt.Printf("[WARN] Failed to send CmdGamesUpdate to %s (channel full?)\n", name)
				}

				if player.Game != "" {
					startPayload := map[string]any{
						"game":        player.Game,
						"instance_id": player.InstanceID,
					}
					select {
					case client.sendCh <- types.Command{
						Cmd:     types.CmdStart,
						Payload: startPayload,
						ID:      fmt.Sprintf("init-%d", time.Now().UnixNano()),
					}:

					default:
						fmt.Printf("[WARN] Failed to send CmdStart to %s (channel full?)\n", name)
					}
				}

				select {
				case client.sendCh <- types.Command{
					Cmd:     types.CmdPing,
					Payload: fmt.Sprintf("%d", time.Now().UnixNano()),
					ID:      fmt.Sprintf("ping-%d", time.Now().UnixNano()),
				}:

				default:
					fmt.Printf("[WARN] Failed to send CmdPing to %s (channel full?)\n", name)
				}
			} else {
				fmt.Printf("[ERROR] Invalid payload type for CmdHello: %T\n", cmd.Payload)
			}
			continue
		}

		log.Printf("client message: %+v", cmd)
	}
}

// sendToPlayer enqueues a command to a player's websocket send queue.
func (s *Server) sendToPlayer(player string, cmd types.Command) error {
	var client *wsClient
	var ok bool
	s.withRLock(func() {
		client, ok = s.players[player]
	})
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

// sendAndWait convenience wrapper that registers pending and waits for ack/nack.
func (s *Server) sendAndWait(player string, cmd types.Command, timeout time.Duration) (string, error) {
	ch := make(chan string, 1)
	s.withLock(func() {
		s.pending[cmd.ID] = ch
	})
	defer s.withLock(func() {
		delete(s.pending, cmd.ID)
	})
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

func (s *Server) sendSwap(player string, game string, instanceID string) {
	go func() {
		payload := map[string]string{"game": game}
		if instanceID != "" {
			payload["instance_id"] = instanceID
		}
		cmd := types.Command{
			Cmd:     types.CmdSwap,
			Payload: payload,
			ID:      fmt.Sprintf("swap-%d-%s", time.Now().UnixNano(), player),
		}
		_, _ = s.sendAndWait(player, cmd, 20*time.Second)
	}()
}
