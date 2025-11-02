package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// apiMessagePlayer: POST {player: ..., message: ..., duration: ..., x: ..., y: ..., fontsize: ..., fg: ..., bg: ...}
func (s *Server) apiMessagePlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Player   string `json:"player"`
		Message  string `json:"message"`
		Duration int    `json:"duration,omitempty"`
		X        int    `json:"x,omitempty"`
		Y        int    `json:"y,omitempty"`
		Fontsize int    `json:"fontsize,omitempty"`
		Fg       string `json:"fg,omitempty"`
		Bg       string `json:"bg,omitempty"`
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

	// Set defaults if not provided
	if b.Duration == 0 {
		b.Duration = 3
	}
	if b.X == 0 && b.Y == 0 { // Allow 0,0 but default to 10,10 if both are 0
		b.X = 10
		b.Y = 10
	}
	if b.Fontsize == 0 {
		b.Fontsize = 12
	}
	if b.Fg == "" {
		b.Fg = "#FFFFFF"
	}
	if b.Bg == "" {
		b.Bg = "#000000"
	}

	// Send message command to the specific player
	cmd := types.Command{
		Cmd: types.CmdMessage,
		Payload: map[string]any{
			"message":  b.Message,
			"duration": b.Duration,
			"x":        b.X,
			"y":        b.Y,
			"fontsize": b.Fontsize,
			"fg":       b.Fg,
			"bg":       b.Bg,
		},
		ID: fmt.Sprintf("message-%d-%s", time.Now().UnixNano(), b.Player),
	}
	var player types.Player
	var ok bool
	s.withRLock(func() {
		player, ok = s.state.Players[b.Player]
	})
	if !ok {
		http.Error(w, "player not found", http.StatusNotFound)
		return
	}

	err := s.sendToPlayer(player, cmd)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to send message: %v", err), http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiMessageAll: POST {message: ..., duration: ..., x: ..., y: ..., fontsize: ..., fg: ..., bg: ...}
func (s *Server) apiMessageAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Message  string `json:"message"`
		Duration int    `json:"duration,omitempty"`
		X        int    `json:"x,omitempty"`
		Y        int    `json:"y,omitempty"`
		Fontsize int    `json:"fontsize,omitempty"`
		Fg       string `json:"fg,omitempty"`
		Bg       string `json:"bg,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if b.Message == "" {
		http.Error(w, "missing message", http.StatusBadRequest)
		return
	}

	// Set defaults if not provided
	if b.Duration == 0 {
		b.Duration = 3
	}
	if b.X == 0 && b.Y == 0 { // Allow 0,0 but default to 10,10 if both are 0
		b.X = 10
		b.Y = 10
	}
	if b.Fontsize == 0 {
		b.Fontsize = 12
	}
	if b.Fg == "" {
		b.Fg = "#FFFFFF"
	}
	if b.Bg == "" {
		b.Bg = "#000000"
	}

	// Send message command to all connected players
	cmd := types.Command{
		Cmd: types.CmdMessage,
		Payload: map[string]any{
			"message":  b.Message,
			"duration": b.Duration,
			"x":        b.X,
			"y":        b.Y,
			"fontsize": b.Fontsize,
			"fg":       b.Fg,
			"bg":       b.Bg,
		},
		ID: fmt.Sprintf("message-all-%d", time.Now().UnixNano()),
	}
	s.broadcastToPlayers(cmd)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"result": "ok",
	}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiFullscreenToggle: POST {player: ...}
func (s *Server) apiFullscreenToggle(w http.ResponseWriter, r *http.Request) {
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

	var player types.Player
	var ok bool
	s.withRLock(func() {
		player, ok = s.state.Players[b.Player]
	})
	if !ok {
		http.Error(w, "player not found", http.StatusNotFound)
		return
	}

	// Send fullscreen toggle command to the specific player
	cmd := types.Command{
		Cmd:     types.CmdFullscreenToggle,
		Payload: map[string]any{},
		ID:      fmt.Sprintf("fullscreen-toggle-%d-%s", time.Now().UnixNano(), b.Player),
	}

	err := s.sendToPlayer(player, cmd)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to send fullscreen toggle: %v", err), http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}
