package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// apiCheckPlayerConfig: POST {player: "playername"}
// Triggers the server to send a check_config command to the specified player
func (s *Server) apiCheckPlayerConfig(w http.ResponseWriter, r *http.Request) {
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

	player, ok := s.state.Players[b.Player]
	if !ok {
		http.Error(w, "player not found", http.StatusNotFound)
		return
	}

	if !player.Connected {
		http.Error(w, "player not connected", http.StatusBadRequest)
		return
	}

	// Send check config command to player with config keys to retrieve
	cmd := types.Command{
		Cmd:     types.CmdCheckConfig,
		Payload: map[string]any{"config_keys": s.state.ConfigKeys},
		ID:      fmt.Sprintf("check-config-%d-%s", time.Now().UnixNano(), b.Player),
	}

	if err := s.sendToPlayer(player, cmd); err != nil {
		http.Error(w, "failed to send command: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "command_sent"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiUpdatePlayerConfig: POST {player: "playername", config: "config_content"}
// Sends updated config to the specified player
func (s *Server) apiUpdatePlayerConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Player string `json:"player"`
		Config string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	player, ok := s.state.Players[b.Player]
	if !ok {
		http.Error(w, "player not found", http.StatusNotFound)
		return
	}

	if !player.Connected {
		http.Error(w, "player not connected", http.StatusBadRequest)
		return
	}

	// Send update config command to player with updated values
	cmd := types.Command{
		Cmd:     types.CmdUpdateConfig,
		Payload: map[string]any{"config_updates": b.Config},
		ID:      fmt.Sprintf("update-config-%d-%s", time.Now().UnixNano(), b.Player),
	}

	if err := s.sendToPlayer(player, cmd); err != nil {
		http.Error(w, "failed to send command: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "command_sent"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiSetConfigKeys: POST {config_keys: ["key1", "key2", ...]}
// Updates the server's config keys array
func (s *Server) apiSetConfigKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		ConfigKeys []string `json:"config_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	s.UpdateStateAndPersist(func(st *types.ServerState) {
		st.ConfigKeys = b.ConfigKeys
	})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "config_keys_updated"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}
