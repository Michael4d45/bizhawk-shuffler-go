package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
		for name, plugin := range s.state.Plugins {
			plugins[name] = plugin
		}
	})

	// Scan plugins directory for any new plugins not in state
	pluginsDir := "./plugins/available"
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

// handlePluginUpload handles plugin file uploads
func (s *Server) handlePluginUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("plugin")
	if err != nil {
		http.Error(w, "plugin file missing: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	// For now, just save as a simple .lua file in the plugins/available directory
	// TODO: Support proper plugin packages with metadata
	if !strings.HasSuffix(header.Filename, ".lua") {
		http.Error(w, "only .lua files supported currently", http.StatusBadRequest)
		return
	}

	pluginName := strings.TrimSuffix(header.Filename, ".lua")
	pluginDir := filepath.Join("./plugins/available", pluginName)

	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		http.Error(w, "failed to create plugin dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	dstPath := filepath.Join(pluginDir, "plugin.lua")
	out, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, "failed to create plugin file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, "failed to save plugin: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create basic metadata file
	metadata := types.Plugin{
		Name:        pluginName,
		Version:     "1.0.0",
		Description: "Uploaded plugin",
		Author:      "Unknown",
		Enabled:     false,
		EntryPoint:  "plugin.lua",
		Status:      types.PluginStatusDisabled,
		Path:        pluginDir,
	}

	metaPath := filepath.Join(pluginDir, "meta.json")
	metaFile, err := os.Create(metaPath)
	if err == nil {
		defer func() {
			if closeErr := metaFile.Close(); closeErr != nil {
				log.Printf("close metaFile error: %v", closeErr)
			}
		}()
		if encodeErr := json.NewEncoder(metaFile).Encode(metadata); encodeErr != nil {
			log.Printf("encode metadata error: %v", encodeErr)
		}
	}

	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, "Plugin %s uploaded successfully", pluginName); err != nil {
		log.Printf("write response error: %v", err)
	}
}

// loadPluginMetadata loads plugin metadata from disk
func (s *Server) loadPluginMetadata(pluginName string) *types.Plugin {
	pluginDir := filepath.Join("./plugins/available", pluginName)
	metaPath := filepath.Join(pluginDir, "meta.json")

	metaFile, err := os.Open(metaPath)
	if err != nil {
		// If no meta.json, create a basic plugin entry
		return &types.Plugin{
			Name:        pluginName,
			Version:     "unknown",
			Description: "Plugin without metadata",
			Author:      "Unknown",
			Enabled:     false,
			EntryPoint:  "plugin.lua",
			Status:      types.PluginStatusDisabled,
			Path:        pluginDir,
		}
	}
	defer func() {
		if err := metaFile.Close(); err != nil {
			log.Printf("close metaFile error: %v", err)
		}
	}()

	var plugin types.Plugin
	if err := json.NewDecoder(metaFile).Decode(&plugin); err != nil {
		return nil
	}

	plugin.Path = pluginDir
	if plugin.Status == "" {
		plugin.Status = types.PluginStatusDisabled
	}

	return &plugin
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
						// prefer state for Enabled/Status and Path
						p.Enabled = sp.Enabled
						if sp.Status != "" {
							p.Status = sp.Status
						}
						if sp.Path != "" {
							p.Path = sp.Path
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
			pluginDir := filepath.Join("./plugins/available", pluginName)
			// Remove directory on disk
			if err := os.RemoveAll(pluginDir); err != nil {
				log.Printf("failed to remove plugin dir %s: %v", pluginDir, err)
				http.Error(w, "failed to remove plugin: "+err.Error(), http.StatusInternalServerError)
				return
			}
			_ = s.UpdateStateAndPersist(func(st *types.ServerState) {
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
	case "enable":
		s.handlePluginEnableByName(w, r, pluginName)
	case "disable":
		s.handlePluginDisableByName(w, r, pluginName)
	default:
		http.Error(w, "unknown action: "+action, http.StatusBadRequest)
	}
}

// handlePluginEnableByName enables a plugin by name
func (s *Server) handlePluginEnableByName(w http.ResponseWriter, r *http.Request, pluginName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	_ = s.UpdateStateAndPersist(func(st *types.ServerState) {
		if st.Plugins == nil {
			st.Plugins = make(map[string]types.Plugin)
		}

		// Load plugin metadata if not already loaded
		plugin, exists := st.Plugins[pluginName]
		if !exists {
			if p := s.loadPluginMetadata(pluginName); p != nil {
				plugin = *p
			}
		}

		plugin.Enabled = true
		plugin.Status = types.PluginStatusEnabled
		st.Plugins[pluginName] = plugin
	})
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		log.Printf("write response error: %v", err)
	}
}

// handlePluginDisableByName disables a plugin by name
func (s *Server) handlePluginDisableByName(w http.ResponseWriter, r *http.Request, pluginName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var exists bool
	_ = s.UpdateStateAndPersist(func(st *types.ServerState) {
		if st.Plugins == nil {
			st.Plugins = make(map[string]types.Plugin)
		}
		// Avoid shadowing the outer 'exists' variable. Use a local ok and set
		// the outer flag so it is visible after the closure.
		plugin, ok := st.Plugins[pluginName]
		if ok {
			exists = true
		} else {
			if p := s.loadPluginMetadata(pluginName); p != nil {
				plugin = *p
				exists = true
			}
		}
		if !exists {
			return
		}
		// Update status to disabled
		plugin.Enabled = false
		plugin.Status = types.PluginStatusDisabled
		st.Plugins[pluginName] = plugin
	})
	if !exists {
		http.Error(w, "plugin not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		log.Printf("write response error: %v", err)
	}
}
