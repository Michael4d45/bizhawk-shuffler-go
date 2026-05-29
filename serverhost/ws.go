package serverhost

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
	"github.com/michael4d45/bizshuffle/obslog"
	"github.com/michael4d45/bizshuffle/protocol"
)

// wsClient represents a connected websocket client and its outbound send queue
type wsClient struct {
	conn   *websocket.Conn
	sendCh chan protocol.Command
}

const wsWriterDrainWait = 2 * time.Second

// handleWS upgrades to websocket and manages client lifecycle.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.wsActive.Add(1)

	c, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		s.wsActive.Done()
		return
	}
	client := &wsClient{conn: c, sendCh: make(chan protocol.Command, 256)}
	s.liveConns.Store(c, client)
	s.withConnLock(func() {
		s.conns[c] = client
	})

	go func() {
		<-ctx.Done()
		_ = c.Close()
	}()

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
			var name string
			s.withConnRLock(func() {
				name = s.findPlayerNameForClientLocked(client)
			})
			if name != "" {
				s.UpdateStateAndPersist(func(st *protocol.ServerState) {
					pl := st.Players[name]
					pl.PingMs = int(rtt.Milliseconds())
					st.Players[name] = pl
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
			case <-ctx.Done():
				return
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
				if cmd.Cmd == protocol.CmdPing {
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
		writerDone := make(chan struct{})
		go func() {
			writeWG.Wait()
			close(writerDone)
		}()
		select {
		case <-writerDone:
		case <-time.After(wsWriterDrainWait):
			log.Printf("ws: writer drain timeout")
		}
		s.wsActive.Done()
		s.removeWSClient(c, client)
		if err := c.Close(); err != nil {
			log.Printf("websocket close error: %v", err)
		}
	}()

	for {
		var cmd protocol.Command
		if err := c.ReadJSON(&cmd); err != nil {
			log.Printf("read: %v", err)
			break
		}
		log.Printf("received cmd from client: %s id=%s", cmd.Cmd, cmd.ID)

		switch cmd.Cmd {
		case protocol.CmdAck, protocol.CmdNack:
			var ch chan string
			var ok bool
			s.withRLock(func() {
				ch, ok = s.pending[cmd.ID]
			})
			if ok {
				if cmd.Cmd == protocol.CmdAck {
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
			} else {
				// Log nack/ack for commands not in pending map (like async responses)
				if cmd.Cmd == protocol.CmdNack {
					log.Printf("received nack for non-pending command id=%s payload=%+v", cmd.ID, cmd.Payload)
				}
			}
			continue
		case protocol.CmdGamesUpdateAck:
			// determine player name and update under locks
			name := ""
			s.withConnRLock(func() {
				name = s.findPlayerNameForClientLocked(client)
			})
			if name != "" {
				if pl, ok := cmd.Payload.(map[string]any); ok {
					if hf, ok := pl["has_files"].(bool); ok {
						s.UpdateStateAndPersist(func(st *protocol.ServerState) {
							p := st.Players[name]
							p.HasFiles = hf
							st.Players[name] = p
						})
						continue
					}
				}
			}
			continue
		case protocol.CmdHello:
			if pl, ok := cmd.Payload.(map[string]any); ok {
				name := ""
				if v, ok := pl["name"].(string); ok {
					name = v
				}
				if name == "" {
					log.Printf("CmdHello missing name in payload")
					continue
				}
				bizhawkReady := false
				if v, ok := pl["bizhawk_ready"].(bool); ok {
					bizhawkReady = v
				}
				s.withConnLock(func() {
					s.conns[c] = client
					s.playerClients[name] = client
				})
				s.UpdateStateAndPersist(func(st *protocol.ServerState) {
					if st.Players == nil {
						st.Players = make(map[string]protocol.Player)
					}
					p, ok := st.Players[name]
					if !ok {
						p = protocol.Player{Name: name}
					}
					p.Connected = true
					p.BizhawkReady = bizhawkReady
					st.Players[name] = p
				})

				player := s.AssignPlayerOnConnect(name)
				player.Connected = true
				player.BizhawkReady = bizhawkReady

				s.broadcastGamesUpdate(&player)
				if player.Game != "" && bizhawkReady {
					s.sendSwap(player, SwapSendOptions{SkipSave: true})
				} else if bizhawkReady && player.Game == "" {
					log.Printf("[ws] hello from %q with bizhawk_ready but no game/instance assigned", name)
					obslog.Event(obslog.Swap, "skip_no_assignment", map[string]string{
						"player": name, "reason": "hello_bizhawk_ready_no_game",
					})
				} else if !bizhawkReady && player.Game == "" {
					log.Printf("[ws] hello from %q (bizhawk_ready=false); swap deferred until Lua HELLO / status_update", name)
					obslog.Event(obslog.Swap, "deferred", map[string]string{
						"player": name, "reason": "hello_bizhawk_not_ready",
					})
				}
				if err := s.sendPing(player); err != nil {
					log.Printf("failed to send ping to player %s: %v", player.Name, err)
				}
			} else {
				fmt.Printf("[ERROR] Invalid payload type for CmdHello: %T\n", cmd.Payload)
			}
			continue
		case protocol.CmdStatusUpdate:
			if pl, ok := cmd.Payload.(map[string]any); ok {
				name := ""
				s.withConnRLock(func() {
					name = s.findPlayerNameForClientLocked(client)
				})
				if name == "" {
					continue
				}
				bizhawkReady, hasReady := pl["bizhawk_ready"].(bool)
				if !hasReady {
					continue
				}
				becameReady := false
				s.UpdateStateAndPersist(func(st *protocol.ServerState) {
					p, ok := st.Players[name]
					if !ok {
						return
					}
					becameReady = bizhawkReady && !p.BizhawkReady
					p.BizhawkReady = bizhawkReady
					st.Players[name] = p
				})
				if becameReady {
					player := s.AssignPlayerOnConnect(name)
					player.Connected = true
					player.BizhawkReady = true
					if player.Game != "" && s.ShouldSendSwap(player, false) {
						s.sendSwap(player, SwapSendOptions{SkipSave: true})
					} else if player.Game == "" {
						log.Printf("[ws] player %q bizhawk ready but no game/instance assigned — configure games in admin before join", name)
						obslog.Event(obslog.Swap, "skip_no_assignment", map[string]string{
							"player": name, "reason": "status_update_bizhawk_ready_no_game",
						})
					}
				}
			}
			continue
		case protocol.CmdHelloAdmin:
			if pl, ok := cmd.Payload.(map[string]any); ok {
				name := ""
				if v, ok := pl["name"].(string); ok {
					name = v
				}
				if name == "" {
					log.Printf("CmdHelloAdmin missing name in payload")
					continue
				}

				s.withConnLock(func() {
					s.conns[c] = client
					s.adminClients[name] = client
				})

				log.Printf("Admin %s connected", name)

				// Send initial ping to establish connection
				select {
				case client.sendCh <- protocol.Command{Cmd: protocol.CmdPing, Payload: fmt.Sprintf("%d", time.Now().UnixNano()), ID: fmt.Sprintf("ping-%d", time.Now().UnixNano())}:
				case <-time.After(5 * time.Second):
					fmt.Printf("[ERROR] Failed to send CmdPing to admin %s (queue full after 5s)\n", name)
				}
			} else {
				fmt.Printf("[ERROR] Invalid payload type for CmdHelloAdmin: %T\n", cmd.Payload)
			}
			continue
		case protocol.CmdTypeLua:
			if pl, ok := cmd.Payload.(map[string]any); ok {
				var luaCmd protocol.LuaCommand
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
				case protocol.LuaCmdMessage:
					// Broadcast message to all players and admins
					s.broadcastToPlayers(protocol.Command{
						Cmd:     protocol.CmdTypeLua,
						Payload: luaCmd,
					})
				case protocol.LuaCmdSwap:
					// Handle swap command
					if err := s.performSwap(); err != nil {
						fmt.Printf("performSwap error: %v\n", err)
					}
				case protocol.LuaCmdSwapMe:
					name := ""
					s.withConnRLock(func() {
						name = s.findPlayerNameForClientLocked(client)
					})
					if name == "" {
						fmt.Printf("[ERROR] LuaCmdSwapMe: could not determine player name for client\n")
						continue
					}
					if err := s.performRandomSwapForPlayer(name); err != nil {
						fmt.Printf("performRandomSwapForPlayer error: %v\n", err)
					}
				}
			} else {
				fmt.Printf("[ERROR] Invalid payload type for CmdTypeLua: %T\n", cmd.Payload)
			}
			continue
		case protocol.CmdConfigResponse:
			// Handle config response from client
			if pl, ok := cmd.Payload.(map[string]any); ok {
				name := ""
				s.withConnRLock(func() {
					name = s.findPlayerNameForClientLocked(client)
				})
				if name != "" {
					if configValues, ok := pl["config_values"].(map[string]any); ok {
						// Store config values on the player state
						s.UpdateStateAndPersist(func(st *protocol.ServerState) {
							if player, exists := st.Players[name]; exists {
								player.ConfigValues = configValues
								st.Players[name] = player
							}
						})
					}
				}
			}
			continue
		default:
			log.Printf("client message: %+v", cmd)
		}
	}
}

// broadcastToPlayers sends a command to all currently connected players.
func (s *Server) broadcastToPlayers(cmd protocol.Command) {
	clients := make([]*wsClient, 0, len(s.playerClients))
	s.withConnRLock(func() {
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
func (s *Server) broadcastToAdmins(cmd protocol.Command) {
	clients := make([]*wsClient, 0, len(s.adminClients))
	s.withConnRLock(func() {
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

func (s *Server) broadcastGamesUpdate(player *protocol.Player) {
	games, mainGames, gameInstances := s.SnapshotGames()
	payload := map[string]any{
		"game_instances": gameInstances,
		"main_games":     mainGames,
		"games":          games,
	}
	if player != nil {
		errs := s.sendToPlayer(*player, protocol.Command{Cmd: protocol.CmdGamesUpdate, Payload: payload, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
		if errs != nil {
			log.Printf("failed to send games update to player %s: %v", player.Name, errs)
		}
	} else {
		s.broadcastToPlayers(protocol.Command{Cmd: protocol.CmdGamesUpdate, Payload: payload, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	}
}

// removeWSClient unregisters a websocket client. Connection maps use connMu; player state uses UpdateStateAndPersist.
func (s *Server) removeWSClient(conn *websocket.Conn, client *wsClient) {
	s.liveConns.Delete(conn)

	var playerName, adminName string
	s.withConnLock(func() {
		cl, ok := s.conns[conn]
		if !ok || cl != client {
			return
		}
		playerName = s.findPlayerNameForClientLocked(cl)
		adminName = s.findAdminNameForClientLocked(cl)
		if playerName != "" {
			delete(s.playerClients, playerName)
		} else if adminName != "" {
			delete(s.adminClients, adminName)
		}
		delete(s.conns, conn)
	})

	if playerName != "" {
		s.UpdateStateAndPersist(func(st *protocol.ServerState) {
			pl := st.Players[playerName]
			pl.Connected = false
			pl.BizhawkReady = false
			st.Players[playerName] = pl
			s.clearPendingForPlayer(st, playerName)
		})
		s.ClearAppliedSwap(playerName)
	} else if adminName != "" {
		log.Printf("Admin %s disconnected", adminName)
	}
}

const closeWebSocketsWait = 2 * time.Second

// CloseWebSockets closes all active websocket connections so HTTP shutdown can finish.
// Upgraded /ws handlers do not exit on Server.Shutdown alone; callers must close conns first.
// Connections are closed without holding s.mu: handleWS defer calls UpdateStateAndPersist
// which also takes the write lock — closing while locked deadlocks shutdown.
func (s *Server) CloseWebSockets() {
	s.closeWebSocketsBounded(closeWebSocketsWait)
}

func (s *Server) closeWebSocketsBounded(wait time.Duration) {
	var toClose []*websocket.Conn
	s.liveConns.Range(func(key, _ any) bool {
		toClose = append(toClose, key.(*websocket.Conn))
		return true
	})
	if len(toClose) == 0 {
		return
	}
	done := make(chan struct{})
	go func() {
		for _, conn := range toClose {
			_ = conn.SetReadDeadline(time.Now())
			_ = conn.Close()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(wait):
		log.Printf("serverhost: websocket close timed out (%d connections)", len(toClose))
	}
}

// sendToPlayer enqueues a command to a player's websocket send queue.
func (s *Server) sendToPlayer(player protocol.Player, cmd protocol.Command) error {
	var client *wsClient
	var ok bool
	s.withConnRLock(func() {
		client, ok = s.playerClients[player.Name]
	})
	if !ok || client == nil {
		return fmt.Errorf("no connection for player %s", player.Name)
	}

	s.broadcastToAdmins(protocol.Command{
		Cmd:     cmd.Cmd,
		Payload: map[string]any{"player": player.Name, "original_payload": cmd.Payload},
		ID:      cmd.ID,
	})

	return enqueueWSCommand(client.sendCh, cmd, 5*time.Second, fmt.Sprintf("player %s", player.Name))
}

func enqueueWSCommand(ch chan protocol.Command, cmd protocol.Command, timeout time.Duration, label string) error {
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%s disconnected", label)
			}
		}()
		select {
		case ch <- cmd:
		case <-time.After(timeout):
			err = fmt.Errorf("send queue full for %s (timeout after %s)", label, timeout)
		}
	}()
	return err
}

func (s *Server) sendPing(player protocol.Player) error {
	var client *wsClient
	var ok bool
	s.withConnRLock(func() {
		client, ok = s.playerClients[player.Name]
	})
	if !ok || client == nil {
		return fmt.Errorf("no connection for player %s", player.Name)
	}
	ping := protocol.Command{Cmd: protocol.CmdPing, Payload: fmt.Sprintf("%d", time.Now().UnixNano()), ID: fmt.Sprintf("ping-%d", time.Now().UnixNano())}
	return enqueueWSCommand(client.sendCh, ping, 5*time.Second, fmt.Sprintf("player %s", player.Name))
}

// broadcastPluginSettingsUpdate broadcasts plugin settings changes to all connected clients
func (s *Server) broadcastPluginSettingsUpdate(pluginName string, settings map[string]string) {
	payload := map[string]any{
		"plugin_name": pluginName,
		"settings":    settings,
	}
	cmd := protocol.Command{
		Cmd:     protocol.CmdStateUpdate,
		Payload: payload,
		ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	s.broadcastToPlayers(cmd)
}

// sendAndWait convenience wrapper that registers pending and waits for ack/nack.
func (s *Server) sendAndWait(player protocol.Player, cmd protocol.Command, timeout time.Duration) (string, error) {
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

func (s *Server) sendSwap(player protocol.Player, opts SwapSendOptions) {
	if player.Game == "" {
		log.Printf("[swap] skip %s: no game in player state", player.Name)
		obslog.Event(obslog.Swap, "skip", map[string]string{
			"player": player.Name, "reason": "no_game",
		})
		return
	}
	if !s.ShouldSendSwap(player, opts.Force) {
		log.Printf("[swap] skip %s: target unchanged (game=%q instance=%q)", player.Name, player.Game, player.InstanceID)
		obslog.Event(obslog.Swap, "skip", map[string]string{
			"player": player.Name, "reason": "unchanged",
			"game":   player.Game, "instance_id": player.InstanceID,
		})
		return
	}
	skip := false
	s.withLock(func() {
		if _, busy := s.swapInFlight[player.Name]; busy {
			skip = true
			return
		}
		s.swapInFlight[player.Name] = struct{}{}
	})
	if skip {
		log.Printf("[swap] skip %s: another swap in flight", player.Name)
		obslog.Event(obslog.Swap, "skip", map[string]string{
			"player": player.Name, "reason": "in_flight",
		})
		return
	}

	go func(p protocol.Player, o SwapSendOptions) {
		defer s.withLock(func() { delete(s.swapInFlight, p.Name) })

		payload := map[string]any{"game": p.Game}
		if p.InstanceID != "" {
			payload["instance_id"] = p.InstanceID
		}
		if o.SkipSave {
			payload["skip_save"] = true
		}
		cmd := protocol.Command{
			Cmd:     protocol.CmdSwap,
			Payload: payload,
			ID:      fmt.Sprintf("swap-%d-%s", time.Now().UnixNano(), p.Name),
		}
		log.Printf("[swap] -> %s game=%q instance=%q skip_save=%v", p.Name, p.Game, p.InstanceID, o.SkipSave)
		obslog.Event(obslog.Swap, "send", map[string]string{
			"player":      p.Name,
			"game":        p.Game,
			"instance_id": p.InstanceID,
			"skip_save":   fmt.Sprintf("%v", o.SkipSave),
		})
		res, err := s.sendAndWait(p, cmd, 20*time.Second)
		if err == nil && res == "ack" {
			s.recordSwapApplied(p.Name, p)
		}
	}(player, opts)
}

func (s *Server) sendSwapAll(opts SwapSendOptions) {
	playersMap := map[string]protocol.Player{}
	s.withRLock(func() {
		maps.Copy(playersMap, s.state.Players)
	})

	for _, p := range playersMap {
		if !p.Connected {
			continue
		}
		s.sendSwap(p, opts)
	}
}

func (s *Server) setPlayerFilePending(player protocol.Player) {
	if !player.Connected || player.InstanceID == "" {
		return
	}
	s.setInstanceFileStateWithPlayer(player.InstanceID, protocol.FileStatePending, player.Name)
}

func (s *Server) SetPendingAllFiles() {
	players := []protocol.Player{}
	s.withRLock(func() {
		for _, p := range s.state.Players {
			players = append(players, p)
		}
	})
	for _, p := range players {
		s.setPlayerFilePending(p)
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
	cmd := protocol.Command{
		Cmd:     protocol.CmdRequestSave,
		Payload: payload,
		ID:      fmt.Sprintf("request-save-%d-%s", time.Now().UnixNano(), playerName),
	}

	return s.sendToPlayer(player, cmd)
}
