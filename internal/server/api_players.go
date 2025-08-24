package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// apiSwapPlayer: POST {player:..., game:...}
func (s *Server) apiSwapPlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Player string `json:"player"`
		Game   string `json:"game"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	cmdID := fmt.Sprintf("swap-%d-%s", time.Now().UnixNano(), b.Player)
	cmd := types.Command{Cmd: types.CmdSwap, ID: cmdID, Payload: map[string]string{"game": b.Game}}
	res, err := s.sendAndWait(b.Player, cmd, 20*time.Second)
	if err != nil {
		if errors.Is(err, ErrTimeout) {
			http.Error(w, "timeout", http.StatusGatewayTimeout)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"result": res})
}

// apiRemovePlayer: POST {player: ...}
func (s *Server) apiRemovePlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Player string `json:"player"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if b.Player == "" {
		http.Error(w, "missing player", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	delete(s.state.Players, b.Player)
	if cl, ok := s.players[b.Player]; ok {
		for c, client := range s.conns {
			if client == cl {
				delete(s.conns, c)
				_ = c.Close()
				break
			}
		}
		delete(s.players, b.Player)
	}
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	s.saveState()
	s.broadcast(types.Command{Cmd: types.CmdStateUpdate, Payload: map[string]any{"updated_at": s.state.UpdatedAt}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
}

// apiSwapAllToGame: POST {game:...}
func (s *Server) apiSwapAllToGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Game string `json:"game"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	players := []string{}
	for name := range s.state.Players {
		players = append(players, name)
	}
	s.mu.Unlock()
	results := map[string]string{}
	for _, p := range players {
		cmdID := fmt.Sprintf("swap-%d-%s", time.Now().UnixNano(), p)
		cmd := types.Command{Cmd: types.CmdSwap, ID: cmdID, Payload: map[string]string{"game": b.Game}}
		res, err := s.sendAndWait(p, cmd, 20*time.Second)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				results[p] = "timeout"
			} else {
				results[p] = "send_failed: " + err.Error()
			}
			continue
		}
		results[p] = res
	}
	json.NewEncoder(w).Encode(results)
}
