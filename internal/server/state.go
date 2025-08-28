package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// loadState loads persisted server state from disk if present.
func (s *Server) loadState() {
	f, err := os.Open("state.json")
	if err != nil {
		log.Printf("state file not found, using defaults: %v", err)
		return
	}
	defer func() { _ = f.Close() }()

	var tmp types.ServerState
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tmp); err != nil {
		log.Printf("failed to decode state file %s: %v", "state.json", err)
		return
	}
	if tmp.GameSwapInstances == nil {
		tmp.GameSwapInstances = []types.GameSwapInstance{}
	}
	// Initialize FileState for existing instances that don't have it set
	for i, instance := range tmp.GameSwapInstances {
		if instance.FileState == "" {
			tmp.GameSwapInstances[i].FileState = types.FileStateNone
		}
	}
	if tmp.Games == nil {
		tmp.Games = []string{}
	}
	if tmp.MainGames == nil {
		tmp.MainGames = []types.GameEntry{}
	}
	if tmp.Players == nil {
		tmp.Players = map[string]types.Player{}
	}
	if tmp.UpdatedAt.IsZero() {
		tmp.UpdatedAt = time.Now()
	}
	for name, player := range tmp.Players {
		player.Connected = false
		tmp.Players[name] = player
	}
	s.mu.Lock()
	s.state = tmp
	s.mu.Unlock()
	log.Printf("loaded state from %s", "state.json")
}

// saveState writes current state atomically to disk.
func (s *Server) saveState() error {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	dir := filepath.Dir("state.json")
	if dir == "" || dir == "." {
		dir = "."
	}
	tmpFile, err := os.CreateTemp(dir, "state-*.tmp")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(tmpFile)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&st); err != nil {
		if err2 := tmpFile.Close(); err2 != nil {
			log.Printf("close tmp file error: %v", err2)
		}
		_ = os.Remove(tmpFile.Name())
		return err
	}
	_ = tmpFile.Sync()
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFile.Name())
		log.Printf("close tmp file error: %v", err)
		return err
	}
	if err := os.Rename(tmpFile.Name(), "state.json"); err != nil {
		_ = os.Remove(tmpFile.Name())
		log.Printf("rename tmp file error: %v", err)
		return err
	}
	return nil
}

// handleStateJSON returns the server state as JSON.
func (s *Server) handleStateJSON(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	// Return an envelope with the persisted state runtime map.
	out := map[string]any{
		"state": st,
	}
	if err := json.NewEncoder(w).Encode(out); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

func (s *Server) GetGameForPlayer(player string) types.Player {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Use direct map lookup by player key. This is deterministic and
	// avoids relying on the Player.Name field matching the map key.
	if pp, ok := s.state.Players[player]; ok {
		return pp
	}
	return types.Player{
		Name: player,
	}
}
