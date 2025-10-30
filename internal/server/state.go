package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

func (s *Server) loadJson(filename string, out any) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("close %s error: %v", filename, closeErr)
		}
	}()
	dec := json.NewDecoder(file)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func (s *Server) saveJson(data any, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("close %s error: %v", filename, closeErr)
		}
	}()
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return err
	}
	return file.Sync()
}

// loadKV reads a minimal key=value metadata file into a Plugin struct.
// Format: key = value (one per line). No comments supported. Keys are
// lowercased when parsed. Whitespace around key and value is trimmed.
func (s *Server) loadKV(filename string, out *types.Plugin) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "name":
			out.Name = val
		case "version":
			out.Version = val
		case "description":
			out.Description = val
		case "author":
			out.Author = val
		case "bizhawk_version":
			out.BizHawkVersion = val
		case "entry_point":
			out.EntryPoint = val
		case "status":
			out.Status = types.PluginStatus(val)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

// saveKV writes the plugin metadata in simple key=value format. It writes
// atomically by writing to a tmp file and renaming into place.
func (s *Server) saveKV(data types.Plugin, filename string) error {
	tmp := filename + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	// Write stable ordering
	if _, err := fmt.Fprintln(f, "name = "+data.Name); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, "version = "+data.Version); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, "description = "+data.Description); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, "author = "+data.Author); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, "bizhawk_version = "+data.BizHawkVersion); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, "entry_point = "+data.EntryPoint); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, "status = "+string(data.Status)); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, filename)
}

// loadState loads persisted server state from disk if present.
func (s *Server) loadState() {
	var tmp types.ServerState
	if err := s.loadJson("state.json", &tmp); err != nil {
		if os.IsNotExist(err) {
			log.Printf("no existing state file found, starting fresh")
			tmp = s.SnapshotState()
		} else {
			log.Printf("failed to load state from disk: %v", err)
			return
		}
	}
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
		tmp.GameSwapInstances[i].PendingPlayer = ""
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

	// Load plugins from plugins directory, ignore tmp.Plugins
	tmp.Plugins = make(map[string]types.Plugin)
	pluginsDir := "./plugins"
	if entries, err := os.ReadDir(pluginsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				pluginName := entry.Name()
				if plugin := s.loadPluginMetadata(pluginName); plugin != nil {
					tmp.Plugins[pluginName] = *plugin
					fmt.Println("loaded plugin metadata for", pluginName)
				}
			}
		}
	}

	s.withLock(func() {
		s.state = tmp
	})
	select {
	case s.saveChan <- struct{}{}:
	default:
		// Channel is full, ignore (non-blocking send)
	}
	log.Printf("loaded state from %s", "state.json")
}

// startSaver runs a goroutine that debounces save requests.
// It waits for save signals and delays saving by 500ms to batch rapid updates.
func (s *Server) startSaver() {
	for range s.saveChan {
		s.saveMutex.Lock()
		if s.saveTimer != nil {
			s.saveTimer.Stop()
		}
		s.saveTimer = time.AfterFunc(500*time.Millisecond, func() {
			if err := s.saveState(); err != nil {
				fmt.Printf("failed to persist state: %v\n", err)
			}
			updatedAt := s.SnapshotState().UpdatedAt
			s.broadcastToAdmins(types.Command{
				Cmd:     types.CmdStateUpdate,
				Payload: map[string]any{"updated_at": updatedAt},
				ID:      fmt.Sprintf("%d", updatedAt.UnixNano()),
			})
		})
		s.saveMutex.Unlock()
	}
}

// saveState writes current state atomically to disk.
func (s *Server) saveState() error {
	st := s.SnapshotState()
	for _, plugin := range st.Plugins {
		if plugin.Status == types.PluginStatusError {
			// Don't save state if any plugin is in error state
			return fmt.Errorf("not saving state due to plugin %s", plugin.Name)
		}
		if err := s.savePluginConfig(plugin); err != nil {
			return fmt.Errorf("failed to save plugin config for %s: %w", plugin.Name, err)
		}
	}
	st.Plugins = nil // Don't persist plugins in state.json
	return s.saveJson(st, "state.json")
}

func (s *Server) savePluginConfig(plugin types.Plugin) error {
	pluginDir := filepath.Join("./plugins", plugin.Name)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugin dir: %w", err)
	}
	metaKV := filepath.Join(pluginDir, "meta.kv")

	var existing types.Plugin
	// Only support KV format
	if err := s.loadKV(metaKV, &existing); err != nil {
		return fmt.Errorf("failed to load existing plugin metadata (kv): %w", err)
	}
	existing.Status = plugin.Status
	s.state.Plugins[plugin.Name] = existing
	// Save KV
	if err := s.saveKV(existing, metaKV); err != nil {
		return fmt.Errorf("failed to save kv metadata: %w", err)
	}
	return nil
}

// loadPluginMetadata loads plugin metadata from disk
func (s *Server) loadPluginMetadata(pluginName string) *types.Plugin {
	var plugin types.Plugin
	pluginDir := filepath.Join("./plugins", pluginName)
	metaKV := filepath.Join(pluginDir, "meta.kv")
	// Only KV supported
	if err := s.loadKV(metaKV, &plugin); err != nil {
		fmt.Printf("failed to load plugin metadata for %s: %v\n", pluginName, err)
		return nil
	}

	return &plugin
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
	s.withLock(func() {
		mut(&s.state)
		s.state.UpdatedAt = updatedAt
	})
	select {
	case s.saveChan <- struct{}{}:
	default:
		// Channel is full, ignore (non-blocking send)
	}
}
