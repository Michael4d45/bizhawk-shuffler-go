package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// SaveIndexEntry represents a saved state file for a player and game.
type SaveIndexEntry struct {
	Player string `json:"player"`
	File   string `json:"file"`
	Size   int64  `json:"size"`
	At     int64  `json:"at"`
	Game   string `json:"game"`
}

// loadState loads persisted server state from disk if present.
func (s *Server) loadState() {
	f, err := os.Open(s.stateFile)
	if err != nil {
		log.Printf("state file not found, using defaults: %v", err)
		return
	}
	defer f.Close()

	var tmp types.ServerState
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tmp); err != nil {
		log.Printf("failed to decode state file %s: %v", s.stateFile, err)
		return
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
	s.mu.Lock()
	s.state = tmp
	s.mu.Unlock()
	log.Printf("loaded state from %s", s.stateFile)
}

// saveState writes current state atomically to disk.
func (s *Server) saveState() error {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	dir := filepath.Dir(s.stateFile)
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
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return err
	}
	_ = tmpFile.Sync()
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return err
	}
	if err := os.Rename(tmpFile.Name(), s.stateFile); err != nil {
		os.Remove(tmpFile.Name())
		return err
	}
	return nil
}

// handleStateJSON returns the server state as JSON.
func (s *Server) handleStateJSON(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	st := s.state
	ep := make(map[string]string, len(s.ephemeral))
	for k, v := range s.ephemeral {
		ep[k] = v
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	// Return an envelope with the persisted state and ephemeral runtime map.
	out := map[string]any{
		"state":     st,
		"ephemeral": ep,
	}
	json.NewEncoder(w).Encode(out)
}

// reindexSaves rebuilds ./saves/index.json from on-disk files.
func (s *Server) reindexSaves() {
	idxPath := filepath.Join("./saves", "index.json")
	entries := []SaveIndexEntry{}
	_ = filepath.Walk("./saves", func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel("./saves", p)
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		if len(parts) == 2 {
			entries = append(entries, SaveIndexEntry{Player: parts[0], File: parts[1], Size: info.Size(), At: info.ModTime().Unix(), Game: strings.TrimSuffix(parts[1], ".state")})
		}
		return nil
	})
	if err := os.MkdirAll(filepath.Dir(idxPath), 0755); err != nil {
		return
	}
	ib, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	tmp := idxPath + ".tmp"
	if err := os.WriteFile(tmp, ib, 0644); err != nil {
		return
	}
	_ = os.Rename(tmp, idxPath)
}
