package server

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// handlePluginsList returns a JSON list of all plugins
func (s *Server) handlePluginsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	plugins := make(map[string]types.Plugin)
	s.withRLock(func() {
		maps.Copy(plugins, s.state.Plugins)
	})

	// Scan plugins directory for any new plugins not in state
	pluginsDir := "./plugins"
	if entries, err := os.ReadDir(pluginsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				pluginName := entry.Name()
				if _, exists := plugins[pluginName]; !exists {
					// Try to load plugin metadata
					if plugin := s.loadPluginMetadata(pluginName); plugin != nil {
						plugins[pluginName] = *plugin
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"plugins": plugins}); err != nil {
		log.Printf("encode plugins list error: %v", err)
	}
}

// handlePluginAction routes plugin-specific actions based on URL path
func (s *Server) handlePluginAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/plugins/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "invalid plugin action path", http.StatusBadRequest)
		return
	}

	pluginName := parts[0]

	// If the path is exactly /api/plugins/{name}, support GET (details) and DELETE (remove)
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			// Return plugin metadata (merge persisted state if present)
			p := s.loadPluginMetadata(pluginName)
			if p == nil {
				http.Error(w, "plugin not found", http.StatusNotFound)
				return
			}
			// Merge state (enabled/status) if present
			s.withRLock(func() {
				if s.state.Plugins != nil {
					if sp, ok := s.state.Plugins[pluginName]; ok {
						// prefer state for Status and Path
						if sp.Status != "" {
							p.Status = sp.Status
						}
					}
				}
			})
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(p); err != nil {
				log.Printf("encode plugin detail error: %v", err)
			}
			return
		case http.MethodDelete:
			// Delete plugin files and remove from state
			pluginDir := filepath.Join("./plugins", pluginName)
			// Remove directory on disk
			if err := os.RemoveAll(pluginDir); err != nil {
				log.Printf("failed to remove plugin dir %s: %v", pluginDir, err)
				http.Error(w, "failed to remove plugin: "+err.Error(), http.StatusInternalServerError)
				return
			}
			s.UpdateStateAndPersist(func(st *types.ServerState) {
				if st.Plugins != nil {
					delete(st.Plugins, pluginName)
				}
			})
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte("ok")); err != nil {
				log.Printf("write response error: %v", err)
			}
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}

	// Otherwise treat as action path: /api/plugins/{name}/{action}
	if len(parts) < 2 {
		http.Error(w, "invalid plugin action path", http.StatusBadRequest)
		return
	}

	action := parts[1]

	switch action {
	case "settings":
		s.handlePluginSettings(w, r, pluginName)
	case "reload":
		s.handlePluginReload(w, r, pluginName)
	default:
		http.Error(w, "unknown action: "+action, http.StatusBadRequest)
	}
}

// handlePluginSettings handles GET and POST requests for plugin settings
func (s *Server) handlePluginSettings(w http.ResponseWriter, r *http.Request, pluginName string) {
	switch r.Method {
	case http.MethodGet:
		// Return current settings
		pluginDir := filepath.Join("./plugins", pluginName)
		settingsKV := filepath.Join(pluginDir, "settings.kv")
		settings, err := s.loadSettingsKV(settingsKV)
		if err != nil {
			http.Error(w, "failed to load settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(settings); err != nil {
			log.Printf("encode settings error: %v", err)
		}

	case http.MethodPost:
		// Update settings
		var requestSettings map[string]string
		if err := json.NewDecoder(r.Body).Decode(&requestSettings); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Validate that status is present
		if _, ok := requestSettings["status"]; !ok {
			http.Error(w, "status field is required", http.StatusBadRequest)
			return
		}

		// Validate status value
		status := requestSettings["status"]
		if status != "enabled" && status != "disabled" {
			http.Error(w, "status must be 'enabled' or 'disabled'", http.StatusBadRequest)
			return
		}

		pluginDir := filepath.Join("./plugins", pluginName)
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			http.Error(w, "failed to create plugin dir: "+err.Error(), http.StatusInternalServerError)
			return
		}

		settingsKV := filepath.Join(pluginDir, "settings.kv")
		if err := s.saveSettingsKV(requestSettings, settingsKV); err != nil {
			http.Error(w, "failed to save settings: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Update plugin status in state
		pluginStatus := types.PluginStatus(status)
		s.UpdateStateAndPersist(func(st *types.ServerState) {
			if st.Plugins == nil {
				st.Plugins = make(map[string]types.Plugin)
			}
			plugin, exists := st.Plugins[pluginName]
			if !exists {
				if p := s.loadPluginMetadata(pluginName); p != nil {
					plugin = *p
				} else {
					plugin = types.Plugin{Name: pluginName}
				}
			}
			plugin.Status = pluginStatus
			st.Plugins[pluginName] = plugin
		})

		// Broadcast settings update to connected clients
		s.broadcastPluginSettingsUpdate(pluginName, requestSettings)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			log.Printf("encode response error: %v", err)
		}

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePluginReload broadcasts a reload command to all clients for the specified plugin
func (s *Server) handlePluginReload(w http.ResponseWriter, r *http.Request, pluginName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Broadcast reload command to all connected clients
	cmd := types.Command{
		Cmd: types.CmdPluginReload,
		Payload: map[string]any{
			"plugin_name": pluginName,
		},
		ID: fmt.Sprintf("plugin-reload-%d-%s", time.Now().UnixNano(), pluginName),
	}
	s.broadcastToPlayers(cmd)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("encode response error: %v", err)
	}
}
