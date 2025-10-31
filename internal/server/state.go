package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
// Note: status is NOT written here - it belongs in settings.kv
func (s *Server) saveKV(data types.Plugin, filename string) error {
	tmp := filename + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	// Write stable ordering (excluding status which is in settings.kv)
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
	// Status is now in settings.kv, not meta.kv
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, filename)
}

// loadSettingsKV reads a settings.kv file and returns all key-value pairs as a map.
// Format: key = value (one per line). No comments supported. Keys are lowercased.
// Whitespace around key and value is trimmed.
func (s *Server) loadSettingsKV(filename string) (map[string]string, error) {
	settings := make(map[string]string)
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty map with default status if file doesn't exist
			return map[string]string{"status": "disabled"}, nil
		}
		return nil, err
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
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key != "" {
			settings[key] = val
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	// Ensure status always exists
	if _, exists := settings["status"]; !exists {
		settings["status"] = "disabled"
	}
	return settings, nil
}

// saveSettingsKV writes settings to a settings.kv file. It writes atomically
// by writing to a tmp file and renaming into place. The status key must always exist.
func (s *Server) saveSettingsKV(settings map[string]string, filename string) error {
	// Ensure status always exists
	if _, exists := settings["status"]; !exists {
		settings["status"] = "disabled"
	}
	tmp := filename + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	// Write status first, then other keys in sorted order
	if _, err := fmt.Fprintln(f, "status = "+settings["status"]); err != nil {
		return err
	}
	// Write other keys in sorted order (excluding status)
	keys := make([]string, 0, len(settings))
	for k := range settings {
		if k != "status" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, err := fmt.Fprintf(f, "%s = %s\n", k, settings[k]); err != nil {
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, filename)
}

// migratePluginStatus migrates the status field from meta.kv to settings.kv if needed.
// This is a one-time migration for existing plugins.
func (s *Server) migratePluginStatus(pluginName string) {
	pluginDir := filepath.Join("./plugins", pluginName)
	metaKV := filepath.Join(pluginDir, "meta.kv")
	settingsKV := filepath.Join(pluginDir, "settings.kv")

	// Check if settings.kv already exists - if so, migration is done
	if _, err := os.Stat(settingsKV); err == nil {
		return
	}

	// Check if meta.kv exists
	if _, err := os.Stat(metaKV); os.IsNotExist(err) {
		// No meta.kv, create default settings.kv
		defaultSettings := map[string]string{"status": "disabled"}
		if err := s.saveSettingsKV(defaultSettings, settingsKV); err != nil {
			log.Printf("failed to create default settings.kv for %s: %v", pluginName, err)
		}
		return
	}

	// Load meta.kv to check for status field
	var metaPlugin types.Plugin
	if err := s.loadKV(metaKV, &metaPlugin); err != nil {
		log.Printf("failed to load meta.kv for migration of %s: %v", pluginName, err)
		// Create default settings.kv anyway
		defaultSettings := map[string]string{"status": "disabled"}
		if err := s.saveSettingsKV(defaultSettings, settingsKV); err != nil {
			log.Printf("failed to create default settings.kv for %s: %v", pluginName, err)
		}
		return
	}

	// Extract status from meta.kv
	status := string(metaPlugin.Status)
	if status == "" {
		status = "disabled"
	}

	// Create settings.kv with migrated status
	settings := map[string]string{"status": status}
	if err := s.saveSettingsKV(settings, settingsKV); err != nil {
		log.Printf("failed to create settings.kv for %s: %v", pluginName, err)
		return
	}

	// Remove status from meta.kv if it exists there
	if status != "" {
		// Reload meta.kv to get all fields
		var existing types.Plugin
		if err := s.loadKV(metaKV, &existing); err == nil {
			// Save meta.kv without status field
			existing.Status = "" // Clear status
			if err := s.saveKV(existing, metaKV); err != nil {
				log.Printf("failed to remove status from meta.kv for %s: %v", pluginName, err)
			}
		}
	}

	log.Printf("migrated status for plugin %s from meta.kv to settings.kv", pluginName)
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
				// Migrate status from meta.kv to settings.kv if needed
				s.migratePluginStatus(pluginName)
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
	settingsKV := filepath.Join(pluginDir, "settings.kv")

	// Load existing settings
	settings, err := s.loadSettingsKV(settingsKV)
	if err != nil {
		// If we can't load, create new settings with just status
		settings = make(map[string]string)
	}

	// Update status in settings
	settings["status"] = string(plugin.Status)

	// Save settings to settings.kv
	if err := s.saveSettingsKV(settings, settingsKV); err != nil {
		return fmt.Errorf("failed to save settings.kv: %w", err)
	}

	// Load full plugin metadata from disk (outside lock to avoid deadlock)
	fullPlugin := s.loadPluginMetadata(plugin.Name)
	if fullPlugin == nil {
		fullPlugin = &plugin
	} else {
		fullPlugin.Status = plugin.Status
	}

	// Update in-memory state
	s.withLock(func() {
		if s.state.Plugins == nil {
			s.state.Plugins = make(map[string]types.Plugin)
		}
		s.state.Plugins[plugin.Name] = *fullPlugin
	})

	return nil
}

// loadPluginMetadata loads plugin metadata from disk.
// It loads read-only metadata from meta.kv and settings (including status) from settings.kv.
func (s *Server) loadPluginMetadata(pluginName string) *types.Plugin {
	var plugin types.Plugin
	pluginDir := filepath.Join("./plugins", pluginName)
	metaKV := filepath.Join(pluginDir, "meta.kv")
	settingsKV := filepath.Join(pluginDir, "settings.kv")

	// Load read-only metadata from meta.kv
	if err := s.loadKV(metaKV, &plugin); err != nil {
		fmt.Printf("failed to load plugin metadata for %s: %v\n", pluginName, err)
		// If meta.kv doesn't exist, plugin.Name might be empty, so set it
		if plugin.Name == "" {
			plugin.Name = pluginName
		}
	}

	// Load settings (including status) from settings.kv
	settings, err := s.loadSettingsKV(settingsKV)
	if err != nil {
		fmt.Printf("failed to load plugin settings for %s: %v\n", pluginName, err)
		// Default to disabled if we can't load settings
		plugin.Status = types.PluginStatusDisabled
	} else {
		// Set status from settings
		if statusStr, ok := settings["status"]; ok {
			plugin.Status = types.PluginStatus(statusStr)
		} else {
			plugin.Status = types.PluginStatusDisabled
		}
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
