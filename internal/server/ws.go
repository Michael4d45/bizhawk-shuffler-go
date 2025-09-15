package server

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
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
	for n, pc := range s.playerClients {
		if pc == client {
			return n
		}
	}
	return ""
}

// findAdminNameForClient returns the admin name associated with the given wsClient or
// empty string if none. Caller must hold s.mu if concurrent access is possible.
func (s *Server) findAdminNameForClient(client *wsClient) string {
	for n, ac := range s.adminClients {
		if ac == client {
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
	client := &wsClient{conn: c, sendCh: make(chan types.Command, 256)}
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
				s.UpdateStateAndPersist(func(st *types.ServerState) {
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
		// remove connection and mark player/admin disconnected under lock
		s.UpdateStateAndPersist(func(st *types.ServerState) {
			if cl, ok := s.conns[c]; ok {
				if name := s.findPlayerNameForClient(cl); name != "" {
					pl := st.Players[name]
					pl.Connected = false
					st.Players[name] = pl
					delete(s.playerClients, name)
				} else if adminName := s.findAdminNameForClient(cl); adminName != "" {
					// Handle admin disconnection - could add admin state management here if needed
					delete(s.adminClients, adminName)
					log.Printf("Admin %s disconnected", adminName)
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

		switch cmd.Cmd {
		case types.CmdAck, types.CmdNack:
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
		case types.CmdGamesUpdateAck:
			// determine player name and update under locks
			pname := ""
			s.withRLock(func() {
				pname = s.findPlayerNameForClient(client)
			})
			if pname != "" {
				if pl, ok := cmd.Payload.(map[string]any); ok {
					if hf, ok := pl["has_files"].(bool); ok {
						s.UpdateStateAndPersist(func(st *types.ServerState) {
							p := st.Players[pname]
							p.HasFiles = hf
							st.Players[pname] = p
						})
						continue
					}
				}
			}
			continue
		case types.CmdHello:
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
				s.UpdateStateAndPersist(func(st *types.ServerState) {
					st.Players[name] = player
					s.conns[c] = client
					s.playerClients[name] = client
				})

				s.broadcastGamesUpdate(&player)
				s.sendSwap(player)
				if err := s.sendPing(player); err != nil {
					log.Printf("failed to send ping to player %s: %v", player.Name, err)
				}
			} else {
				fmt.Printf("[ERROR] Invalid payload type for CmdHello: %T\n", cmd.Payload)
			}
			continue
		case types.CmdHelloAdmin:
			if pl, ok := cmd.Payload.(map[string]any); ok {
				name := ""
				if v, ok := pl["name"].(string); ok {
					name = v
				}
				if name == "" {
					log.Printf("CmdHelloAdmin missing name in payload")
					continue
				}

				// Register the admin connection
				s.UpdateStateAndPersist(func(st *types.ServerState) {
					s.conns[c] = client
					s.adminClients[name] = client
				})

				log.Printf("Admin %s connected", name)

				// Send initial ping to establish connection
				select {
				case client.sendCh <- types.Command{Cmd: types.CmdPing, Payload: fmt.Sprintf("%d", time.Now().UnixNano()), ID: fmt.Sprintf("ping-%d", time.Now().UnixNano())}:
				case <-time.After(5 * time.Second):
					fmt.Printf("[ERROR] Failed to send CmdPing to admin %s (queue full after 5s)\n", name)
				}
			} else {
				fmt.Printf("[ERROR] Invalid payload type for CmdHelloAdmin: %T\n", cmd.Payload)
			}
			continue
		case types.CmdTypeLua:
			if pl, ok := cmd.Payload.(map[string]any); ok {
				var luaCmd types.LuaCommand
				b, err := json.Marshal(pl)
				if err != nil {
					log.Printf("failed to marshal lua command payload: %v", err)
					continue
				}
				if err := json.Unmarshal(b, &luaCmd); err != nil {
					log.Printf("failed to unmarshal lua command payload: %v", err)
					continue
				}
				// Handle the Lua command as needed. For now, just log it.
				log.Printf("Received Lua command: kind=%q fields=%v", luaCmd.Kind, luaCmd.Fields)

				switch luaCmd.Kind {
				case types.CmdMessage:
					// Broadcast message to all players and admins
					s.broadcastToPlayers(types.Command{
						Cmd:     types.CmdTypeLua,
						Payload: luaCmd,
					})
				case types.CmdSwap:
					// Handle swap command
					if err := s.performSwap(); err != nil {
						fmt.Printf("performSwap error: %v\n", err)
					}
				}
			} else {
				fmt.Printf("[ERROR] Invalid payload type for CmdTypeLua: %T\n", cmd.Payload)
			}
			continue
		default:
			log.Printf("client message: %+v", cmd)
		}
	}
}

// broadcastToPlayers sends a command to all currently connected players.
func (s *Server) broadcastToPlayers(cmd types.Command) {
	clients := make([]*wsClient, 0, len(s.playerClients))
	s.withRLock(func() {
		for _, cl := range s.playerClients {
			clients = append(clients, cl)
		}
	})
	for _, cl := range clients {
		go func(cl *wsClient) {
			select {
			case cl.sendCh <- cmd:
			case <-time.After(5 * time.Second):
				log.Printf("failed to broadcast to player: queue full")
			}
		}(cl)
	}
	s.broadcastToAdmins(cmd)
}

// broadcastToAdmins sends a command to all currently connected admins.
func (s *Server) broadcastToAdmins(cmd types.Command) {
	clients := make([]*wsClient, 0, len(s.adminClients))
	s.withRLock(func() {
		for _, cl := range s.adminClients {
			clients = append(clients, cl)
		}
	})
	for _, cl := range clients {
		go func(cl *wsClient) {
			select {
			case cl.sendCh <- cmd:
			case <-time.After(5 * time.Second):
				log.Printf("failed to broadcast to admin: queue full")
			}
		}(cl)
	}
}

func (s *Server) broadcastGamesUpdate(player *types.Player) {
	games, mainGames, gameInstances := s.SnapshotGames()
	payload := map[string]any{
		"game_instances": gameInstances,
		"main_games":     mainGames,
		"games":          games,
	}
	if player != nil {
		errs := s.sendToPlayer(*player, types.Command{Cmd: types.CmdGamesUpdate, Payload: payload, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
		if errs != nil {
			log.Printf("failed to send games update to player %s: %v", player.Name, errs)
		}
	} else {
		s.broadcastToPlayers(types.Command{Cmd: types.CmdGamesUpdate, Payload: payload, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	}
}

// sendToPlayer enqueues a command to a player's websocket send queue.
func (s *Server) sendToPlayer(player types.Player, cmd types.Command) error {
	var client *wsClient
	var ok bool
	s.withRLock(func() {
		client, ok = s.playerClients[player.Name]
	})
	if !ok || client == nil {
		return fmt.Errorf("no connection for player %s", player.Name)
	}

	s.broadcastToAdmins(types.Command{
		Cmd:     cmd.Cmd,
		Payload: map[string]any{"player": player.Name, "original_payload": cmd.Payload},
		ID:      cmd.ID,
	})

	select {
	case client.sendCh <- cmd:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("send queue full for player %s (timeout after 5s)", player.Name)
	}
}

func (s *Server) sendPing(player types.Player) error {
	var client *wsClient
	var ok bool
	s.withRLock(func() {
		client, ok = s.playerClients[player.Name]
	})
	if !ok || client == nil {
		return fmt.Errorf("no connection for player %s", player.Name)
	}
	select {
	case client.sendCh <- types.Command{Cmd: types.CmdPing, Payload: fmt.Sprintf("%d", time.Now().UnixNano()), ID: fmt.Sprintf("ping-%d", time.Now().UnixNano())}:
	case <-time.After(5 * time.Second):
		return fmt.Errorf("send queue full for player %s (timeout after 5s)", player.Name)
	}
	return nil
}

// sendAndWait convenience wrapper that registers pending and waits for ack/nack.
func (s *Server) sendAndWait(player types.Player, cmd types.Command, timeout time.Duration) (string, error) {
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

func (s *Server) sendSwap(player types.Player) {
	go func() {
		payload := map[string]string{"game": player.Game}
		if player.InstanceID != "" {
			payload["instance_id"] = player.InstanceID
		}
		cmd := types.Command{
			Cmd:     types.CmdSwap,
			Payload: payload,
			ID:      fmt.Sprintf("swap-%d-%s", time.Now().UnixNano(), player.Name),
		}
		_, _ = s.sendAndWait(player, cmd, 20*time.Second)
	}()
}

func (s *Server) sendSwapAll() {
	// capture local copy of instances for sending without holding lock while network operations run
	playersMap := map[string]types.Player{}
	s.withRLock(func() {
		maps.Copy(playersMap, s.state.Players)
	})

	// Send swap command to each connected player. Include instance_id when player has an assigned instance.
	for _, p := range playersMap {
		if !p.Connected {
			continue
		}
		s.sendSwap(p)
	}
}

func (s *Server) SetPendingAllFiles() {
	players := []types.Player{}
	s.withRLock(func() {
		for _, p := range s.state.Players {
			players = append(players, p)
		}
	})
	for _, p := range players {
		if !p.Connected || p.InstanceID == "" {
			continue
		}
		s.setInstanceFileStateWithPlayer(p.InstanceID, types.FileStatePending, p.Name)
	}
}

// RequestSave sends a request to save command to the specified player for the given instance
func (s *Server) RequestSave(playerName string, instanceID string) error {
	player, ok := s.state.Players[playerName]
	if !ok {
		return fmt.Errorf("player %s not found", playerName)
	}
	if !player.Connected {
		return fmt.Errorf("player %s not connected", playerName)
	}

	payload := map[string]string{"instance_id": instanceID}
	cmd := types.Command{
		Cmd:     types.CmdRequestSave,
		Payload: payload,
		ID:      fmt.Sprintf("request-save-%d-%s", time.Now().UnixNano(), playerName),
	}

	return s.sendToPlayer(player, cmd)
}
