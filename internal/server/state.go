package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

	var tmp types.ServerState
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tmp); err != nil {
		log.Printf("failed to decode state file %s: %v", "state.json", err)
		return
	}
	// Close the file immediately after decoding to avoid locking issues on Windows
	_ = f.Close()
	if tmp.GameSwapInstances == nil {
		tmp.GameSwapInstances = []types.GameSwapInstance{}
	}
	// Initialize FileState for existing instances that don't have it set
	for i, instance := range tmp.GameSwapInstances {
		fmt.Println("checking instance", instance.ID, "file state:", instance.FileState)
		savePath := filepath.Join("./saves", instance.ID+".state")
		if _, err := os.Stat(savePath); err == nil {
			// File exists, mark as ready
			fmt.Println("found save state for instance", instance.ID)
			tmp.GameSwapInstances[i].FileState = types.FileStateReady
		} else {
			// File doesn't exist, mark as none
			fmt.Println("no save state found for instance", instance.ID)
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
	s.withLock(func() {
		s.state = tmp
	})
	if err := s.saveState(); err != nil {
		log.Printf("failed to persist state after load: %v", err)
	}
	log.Printf("loaded state from %s", "state.json")
}

// saveState writes current state atomically to disk.
func (s *Server) saveState() error {
	s.mu.Lock()
	st := s.state
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
	s.mu.Unlock()
	return nil
}

// handleStateJSON returns the server state as JSON.
func (s *Server) handleStateJSON(w http.ResponseWriter, r *http.Request) {
	st := s.SnapshotState()
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
	s.withRLock(func() {}) // ensure lock exists for method shape; body uses state access below via SnapshotState
	// Use direct map lookup by player key. This is deterministic and
	// avoids relying on the Player.Name field matching the map key.
	st := s.SnapshotState()
	if pp, ok := st.Players[player]; ok {
		return pp
	}
	return types.Player{Name: player}
}

// SnapshotState returns a copy of the server state for safe inspection
// outside the lock. It performs a shallow copy of the ServerState value.
func (s *Server) SnapshotState() types.ServerState {
	var st types.ServerState
	s.withRLock(func() {
		st = s.state
	})
	return st
}

// SnapshotPlayers returns a shallow copy of the players map for safe
// iteration without holding the server lock.
func (s *Server) SnapshotPlayers() map[string]types.Player {
	out := make(map[string]types.Player, len(s.state.Players))
	s.withRLock(func() {
		for k, v := range s.state.Players {
			out[k] = v
		}
	})
	return out
}

// SnapshotGames returns shallow copies of games, mainGames and instances
// to allow callers to perform IO/network work without holding locks.
func (s *Server) SnapshotGames() (games []string, mainGames []types.GameEntry, instances []types.GameSwapInstance) {
	s.withRLock(func() {
		games = append([]string(nil), s.state.Games...)
		mainGames = append([]types.GameEntry(nil), s.state.MainGames...)
		instances = append([]types.GameSwapInstance(nil), s.state.GameSwapInstances...)
	})
	return
}

// UpdateStateAndPersist runs a mutator under the write lock, updates the
// UpdatedAt timestamp, then persists the state to disk outside the lock.
// The mutator must not perform blocking IO or call back into server methods
// that attempt to acquire the server lock.
func (s *Server) UpdateStateAndPersist(mut func(*types.ServerState)) {
	updatedAt := time.Now()
	_, file, line, ok := runtime.Caller(1)
	var updatedBy string
	if ok {
		// Make path relative to two directories up
		if wd, err := os.Getwd(); err == nil {
			baseDir := filepath.Dir(filepath.Dir(wd))
			if rel, err := filepath.Rel(baseDir, file); err == nil {
				file = rel
			}
		}
		updatedBy = fmt.Sprintf("%s:%d", file, line)
	} else {
		updatedBy = "unknown"
	}
	s.withLock(func() {
		mut(&s.state)
		s.state.UpdatedAt = updatedAt
	})
	if err := s.saveState(); err != nil {
		fmt.Printf("failed to persist state: %v\n", err)
	}
	s.broadcastToAdmins(types.Command{
		Cmd: types.CmdStateUpdate,
		Payload: map[string]any{
			"updated_at": updatedAt,
			"updated_by": updatedBy,
		},
		ID: fmt.Sprintf("%d", updatedAt.UnixNano()),
	})
}
