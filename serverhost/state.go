package serverhost

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
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
// Also parses setting metadata in format: setting.{key}.type and setting.{key}.options
func (s *Server) loadKV(filename string, out *protocol.Plugin) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if out.SettingsMeta == nil {
		out.SettingsMeta = make(map[string]protocol.SettingMeta)
	}

	return protocol.ForEachKVLine(file, true, func(key, val string) error {
		if strings.HasPrefix(key, "setting.") {
			parts := strings.SplitN(key, ".", 3)
			if len(parts) == 3 {
				settingKey := parts[1]
				metaField := parts[2]

				if _, exists := out.SettingsMeta[settingKey]; !exists {
					out.SettingsMeta[settingKey] = protocol.SettingMeta{}
				}
				meta := out.SettingsMeta[settingKey]

				switch metaField {
				case "type":
					meta.Type = val
				case "options":
					options := strings.Split(val, ",")
					meta.Options = make([]string, 0, len(options))
					for _, opt := range options {
						opt = strings.TrimSpace(opt)
						if opt != "" {
							meta.Options = append(meta.Options, opt)
						}
					}
				}
				out.SettingsMeta[settingKey] = meta
			}
			return nil
		}

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
		}
		return nil
	})
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
	if err := protocol.ForEachKVLine(file, false, func(key, val string) error {
		if key != "" {
			settings[key] = val
		}
		return nil
	}); err != nil {
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

// ensurePluginSettings creates settings.kv with default status when a plugin dir has none.
func (s *Server) ensurePluginSettings(pluginName string) {
	settingsKV := filepath.Join("./plugins", pluginName, "settings.kv")
	if _, err := os.Stat(settingsKV); err == nil {
		return
	}
	defaultSettings := map[string]string{"status": "disabled"}
	if err := s.saveSettingsKV(defaultSettings, settingsKV); err != nil {
		log.Printf("failed to create default settings.kv for %s: %v", pluginName, err)
	}
}

// loadState loads persisted server state from disk if present.
func (s *Server) loadState() {
	var tmp protocol.ServerState
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
		tmp.GameSwapInstances = []protocol.GameSwapInstance{}
	}
	// Initialize FileState for existing instances that don't have it set
	for i, instance := range tmp.GameSwapInstances {
		fmt.Println("checking instance", instance.ID, "file state:", instance.FileState)
		savePath := filepath.Join("./saves", instance.ID+".state")
		if _, err := os.Stat(savePath); err == nil {
			// File exists, mark as ready
			fmt.Println("found save state for instance", instance.ID)
			tmp.GameSwapInstances[i].FileState = protocol.FileStateReady
		} else {
			// File doesn't exist, mark as none
			fmt.Println("no save state found for instance", instance.ID)
			tmp.GameSwapInstances[i].FileState = protocol.FileStateNone
		}
		tmp.GameSwapInstances[i].PendingPlayer = ""
	}
	if tmp.Games == nil {
		tmp.Games = []string{}
	}
	if tmp.MainGames == nil {
		tmp.MainGames = []protocol.GameEntry{}
	}
	if tmp.Players == nil {
		tmp.Players = map[string]protocol.Player{}
	}
	if tmp.ConfigKeys == nil {
		// Initialize default config keys if not set
		tmp.ConfigKeys = []string{
			"DisplayFps",
		}
	}
	tmp.UpdatedAt = time.Now()
	for name, player := range tmp.Players {
		player.Connected = false
		tmp.Players[name] = player
	}

	// Load plugins from plugins directory, ignore tmp.Plugins
	tmp.Plugins = make(map[string]protocol.Plugin)
	pluginsDir := "./plugins"
	if entries, err := os.ReadDir(pluginsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				pluginName := entry.Name()
				s.ensurePluginSettings(pluginName)
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
			s.broadcastToAdmins(protocol.Command{
				Cmd:     protocol.CmdStateUpdate,
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
		if plugin.Status == protocol.PluginStatusError {
			// Don't save state if any plugin is in error state
			return fmt.Errorf("not saving state due to plugin %s", plugin.Name)
		}
		if err := s.savePluginConfig(plugin); err != nil {
			return fmt.Errorf("failed to save plugin config for %s: %w", plugin.Name, err)
		}
	}
	st.Plugins = nil // Don't persist plugins in state.json
	st.UpdatedAt = time.Time{} // Don't persist updated_at (avoids noisy state.json diffs)
	return s.saveJson(st, "state.json")
}

func (s *Server) savePluginConfig(plugin protocol.Plugin) error {
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
			s.state.Plugins = make(map[string]protocol.Plugin)
		}
		s.state.Plugins[plugin.Name] = *fullPlugin
	})

	return nil
}

// loadPluginMetadata loads plugin metadata from disk.
// It loads read-only metadata from meta.kv and settings (including status) from settings.kv.
func (s *Server) loadPluginMetadata(pluginName string) *protocol.Plugin {
	var plugin protocol.Plugin
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
		plugin.Status = protocol.PluginStatusDisabled
	} else {
		// Set status from settings
		if statusStr, ok := settings["status"]; ok {
			plugin.Status = protocol.PluginStatus(statusStr)
		} else {
			plugin.Status = protocol.PluginStatusDisabled
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

func (s *Server) GetGameForPlayer(player string) protocol.Player {
	// Use direct map lookup by player key. This is deterministic and
	// avoids relying on the Player.Name field matching the map key.
	st := s.SnapshotState()
	if pp, ok := st.Players[player]; ok {
		return pp
	}
	return protocol.Player{Name: player}
}

// SnapshotState returns a copy of the server state for safe inspection
// outside the lock. It performs a shallow copy of the ServerState value.
func (s *Server) SnapshotState() protocol.ServerState {
	var st protocol.ServerState
	s.withRLock(func() {
		st = s.state
	})
	return st
}

// SnapshotPlayers returns a shallow copy of the players map for safe
// iteration without holding the server lock.
func (s *Server) SnapshotPlayers() map[string]protocol.Player {
	out := make(map[string]protocol.Player, len(s.state.Players))
	s.withRLock(func() {
		for k, v := range s.state.Players {
			out[k] = v
		}
	})
	return out
}

// SnapshotGames returns shallow copies of games, mainGames and instances
// to allow callers to perform IO/network work without holding locks.
func (s *Server) SnapshotGames() (games []string, mainGames []protocol.GameEntry, instances []protocol.GameSwapInstance) {
	s.withRLock(func() {
		games = append([]string(nil), s.state.Games...)
		mainGames = append([]protocol.GameEntry(nil), s.state.MainGames...)
		instances = append([]protocol.GameSwapInstance(nil), s.state.GameSwapInstances...)
	})
	return
}

// UpdateStateAndPersist runs a mutator under the write lock, updates the
// UpdatedAt timestamp, then persists the state to disk outside the lock.
// The mutator must not perform blocking IO or call back into server methods
// that attempt to acquire the server lock.
func (s *Server) UpdateStateAndPersist(mut func(*protocol.ServerState)) {
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
