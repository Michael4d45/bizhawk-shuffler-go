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

	s.mu.Lock()
	plugins := make(map[string]types.Plugin)
	for name, plugin := range s.state.Plugins {
		plugins[name] = plugin
	}
	s.mu.Unlock()

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

// handlePluginEnable enables a specific plugin
func (s *Server) handlePluginEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pluginName := strings.TrimPrefix(r.URL.Path, "/api/plugins/")
	pluginName = strings.TrimSuffix(pluginName, "/enable")

	if pluginName == "" {
		http.Error(w, "plugin name required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.Plugins == nil {
		s.state.Plugins = make(map[string]types.Plugin)
	}

	// Load plugin metadata if not already loaded
	plugin, exists := s.state.Plugins[pluginName]
	if !exists {
		if p := s.loadPluginMetadata(pluginName); p != nil {
			plugin = *p
		} else {
			http.Error(w, "plugin not found", http.StatusNotFound)
			return
		}
	}

	plugin.Enabled = true
	plugin.Status = types.PluginStatusEnabled
	s.state.Plugins[pluginName] = plugin

	if err := s.saveState(); err != nil {
		log.Printf("saveState error: %v", err)
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		log.Printf("write response error: %v", err)
	}
}

// handlePluginDisable disables a specific plugin
func (s *Server) handlePluginDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pluginName := strings.TrimPrefix(r.URL.Path, "/api/plugins/")
	pluginName = strings.TrimSuffix(pluginName, "/disable")

	if pluginName == "" {
		http.Error(w, "plugin name required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.Plugins == nil {
		s.state.Plugins = make(map[string]types.Plugin)
	}

	plugin, exists := s.state.Plugins[pluginName]
	if !exists {
		http.Error(w, "plugin not found", http.StatusNotFound)
		return
	}

	plugin.Enabled = false
	plugin.Status = types.PluginStatusDisabled
	s.state.Plugins[pluginName] = plugin

	if err := s.saveState(); err != nil {
		log.Printf("saveState error: %v", err)
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		log.Printf("write response error: %v", err)
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
		json.NewEncoder(metaFile).Encode(metadata)
		metaFile.Close()
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Plugin %s uploaded successfully", pluginName)
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
	defer metaFile.Close()

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

	if len(parts) < 2 {
		http.Error(w, "invalid plugin action path", http.StatusBadRequest)
		return
	}

	pluginName := parts[0]
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.Plugins == nil {
		s.state.Plugins = make(map[string]types.Plugin)
	}

	// Load plugin metadata if not already loaded
	plugin, exists := s.state.Plugins[pluginName]
	if !exists {
		if p := s.loadPluginMetadata(pluginName); p != nil {
			plugin = *p
		} else {
			http.Error(w, "plugin not found", http.StatusNotFound)
			return
		}
	}

	plugin.Enabled = true
	plugin.Status = types.PluginStatusEnabled
	s.state.Plugins[pluginName] = plugin

	if err := s.saveState(); err != nil {
		log.Printf("saveState error: %v", err)
	}

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

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.Plugins == nil {
		s.state.Plugins = make(map[string]types.Plugin)
	}

	plugin, exists := s.state.Plugins[pluginName]
	if !exists {
		http.Error(w, "plugin not found", http.StatusNotFound)
		return
	}

	plugin.Enabled = false
	plugin.Status = types.PluginStatusDisabled
	s.state.Plugins[pluginName] = plugin

	if err := s.saveState(); err != nil {
		log.Printf("saveState error: %v", err)
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		log.Printf("write response error: %v", err)
	}
}
