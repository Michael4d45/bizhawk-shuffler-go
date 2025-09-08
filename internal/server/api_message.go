package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// apiMessagePlayer: POST {player: ..., message: ...}
func (s *Server) apiMessagePlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Player  string `json:"player"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if b.Player == "" {
		http.Error(w, "missing player", http.StatusBadRequest)
		return
	}
	if b.Message == "" {
		http.Error(w, "missing message", http.StatusBadRequest)
		return
	}

	// Send message command to the specific player
	cmd := types.Command{
		Cmd:     types.CmdMessage,
		Payload: map[string]string{"message": b.Message},
		ID:      fmt.Sprintf("message-%d-%s", time.Now().UnixNano(), b.Player),
	}

	err := s.sendToPlayer(b.Player, cmd)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to send message: %v", err), http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiMessageAll: POST {message: ...}
func (s *Server) apiMessageAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if b.Message == "" {
		http.Error(w, "missing message", http.StatusBadRequest)
		return
	}

	// Send message command to all connected players
	cmd := types.Command{
		Cmd:     types.CmdMessage,
		Payload: map[string]string{"message": b.Message},
		ID:      fmt.Sprintf("message-all-%d", time.Now().UnixNano()),
	}

	s.mu.Lock()
	players := make([]string, 0, len(s.players))
	for name := range s.players {
		players = append(players, name)
	}
	s.mu.Unlock()

	results := make(map[string]string)
	for _, player := range players {
		err := s.sendToPlayer(player, cmd)
		if err != nil {
			results[player] = "error: " + err.Error()
		} else {
			results[player] = "ok"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"result":  "ok",
		"results": results,
	}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}
