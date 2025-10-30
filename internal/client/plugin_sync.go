package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// PluginSyncManager handles synchronization of plugins between client and server
type PluginSyncManager struct {
	api        *API
	httpClient *http.Client
	config     Config
}

// PluginSyncResult represents the result of a plugin sync operation
type PluginSyncResult struct {
	TotalPlugins int
	Downloaded   int
	Updated      int
	Removed      int
	Errors       []string
	StartTime    time.Time
	Duration     time.Duration
}

// NewPluginSyncManager creates a new plugin sync manager
func NewPluginSyncManager(api *API, httpClient *http.Client, config Config) *PluginSyncManager {
	return &PluginSyncManager{api: api, httpClient: httpClient, config: config}
}

// SyncPlugins orchestrates plugin synchronization.
func (psm *PluginSyncManager) SyncPlugins() (*PluginSyncResult, error) {
	log.Println("=== Starting Plugin Synchronization ===")
	start := time.Now()

	res := &PluginSyncResult{StartTime: start, Errors: []string{}}

	serverPlugins, err := psm.fetchServerPlugins()
	if err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("fetch server plugins: %v", err))
		return res, err
	}
	res.TotalPlugins = len(serverPlugins)

	localPlugins, err := psm.scanLocalPlugins()
	if err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("scan local plugins: %v", err))
		return res, err
	}

	toDownload, toRemove := psm.analyzeSyncRequirements(serverPlugins, localPlugins)

	// Download (always) - downloads overwrite existing files
	for _, name := range toDownload {
		if err := psm.downloadPlugin(name); err != nil {
			log.Printf("ERROR: download %s: %v", name, err)
			res.Errors = append(res.Errors, fmt.Sprintf("download %s: %v", name, err))
		} else {
			res.Downloaded++
		}
	}

	// Remove local plugins that are not enabled on server
	for _, name := range toRemove {
		if err := psm.removeLocalPlugin(name); err != nil {
			log.Printf("ERROR: remove %s: %v", name, err)
			res.Errors = append(res.Errors, fmt.Sprintf("remove %s: %v", name, err))
		} else {
			res.Removed++
		}
	}

	res.Duration = time.Since(start)
	log.Printf("=== Plugin Sync Complete: downloaded=%d removed=%d errors=%d duration=%v ===", res.Downloaded, res.Removed, len(res.Errors), res.Duration)
	return res, nil
}

// fetchServerPlugins retrieves enabled plugins from the server API
func (psm *PluginSyncManager) fetchServerPlugins() (map[string]types.Plugin, error) {
	url := psm.api.BaseURL + "/api/plugins"
	resp, err := psm.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var payload struct {
		Plugins map[string]types.Plugin `json:"plugins"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding plugins JSON: %w", err)
	}

	enabled := make(map[string]types.Plugin)
	for name, p := range payload.Plugins {
		enabled[name] = p
	}
	return enabled, nil
}

// scanLocalPlugins reads the ./plugins directory and returns discovered metadata
func (psm *PluginSyncManager) scanLocalPlugins() (map[string]types.Plugin, error) {
	pluginDir := "./plugins"
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return nil, fmt.Errorf("ensure plugin dir: %w", err)
	}

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil, fmt.Errorf("read plugin dir: %w", err)
	}

	out := make(map[string]types.Plugin)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Only support meta.kv format
		metaKV := filepath.Join(pluginDir, name, "meta.kv")
		var p types.Plugin
		if f, err := os.Open(metaKV); err == nil {
			// parse simple kv file
			scanner := bufio.NewScanner(f)
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
					p.Name = val
				case "version":
					p.Version = val
				case "description":
					p.Description = val
				case "author":
					p.Author = val
				case "bizhawk_version":
					p.BizHawkVersion = val
				case "entry_point":
					p.EntryPoint = val
				case "status":
					p.Status = types.PluginStatus(val)
				}
			}
			_ = f.Close()
			if p.Name == "" {
				p.Name = name
			}
			out[name] = p
			continue
		} else {
			out[name] = types.Plugin{Name: name, Version: "unknown", Status: types.PluginStatusDisabled}
			continue
		}
	}
	return out, nil
}

// analyzeSyncRequirements determines downloads and removals. We intentionally always download enabled plugins.
func (psm *PluginSyncManager) analyzeSyncRequirements(serverPlugins, localPlugins map[string]types.Plugin) (toDownload, toRemove []string) {
	// All enabled plugins should be downloaded
	for name := range serverPlugins {
		toDownload = append(toDownload, name)
	}
	// Remove local plugins that are not enabled on server
	for name := range localPlugins {
		if _, ok := serverPlugins[name]; !ok {
			toRemove = append(toRemove, name)
		}
	}
	return toDownload, toRemove
}

// downloadPlugin fetches plugin.lua and meta.json (when available) and writes them to ./plugins/<name>/
func (psm *PluginSyncManager) downloadPlugin(pluginName string) error {
	base := fmt.Sprintf("%s/files/plugins/%s", psm.api.BaseURL, pluginName)
	localDir := filepath.Join("./plugins", pluginName)
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", localDir, err)
	}

	// helper to download a single file
	downloadFile := func(url, dest string) error {
		resp, err := psm.httpClient.Get(url)
		if err != nil {
			return fmt.Errorf("get %s: %w", url, err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("server returned %d for %s", resp.StatusCode, url)
		}
		tmp := dest + ".tmp"
		f, err := os.Create(tmp)
		if err != nil {
			return fmt.Errorf("create %s: %w", tmp, err)
		}
		if _, err := io.Copy(f, resp.Body); err != nil {
			_ = f.Close()
			return fmt.Errorf("write %s: %w", tmp, err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("close %s: %w", tmp, err)
		}
		if err := os.Rename(tmp, dest); err != nil {
			return fmt.Errorf("rename %s -> %s: %w", tmp, dest, err)
		}
		return nil
	}

	// plugin.lua (required)
	if err := downloadFile(base+"/plugin.lua", filepath.Join(localDir, "plugin.lua")); err != nil {
		return fmt.Errorf("download plugin.lua: %w", err)
	}

	// download meta.kv (required)
	if err := downloadFile(base+"/meta.kv", filepath.Join(localDir, "meta.kv")); err != nil {
		log.Printf("ERROR: meta.kv not available for %s: %v", pluginName, err)
		return fmt.Errorf("meta.kv missing for %s: %w", pluginName, err)
	}

	return nil
}

// removeLocalPlugin deletes the plugin directory
func (psm *PluginSyncManager) removeLocalPlugin(pluginName string) error {
	dir := filepath.Join("./plugins", pluginName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	return nil
}

// GetSyncStatus returns a simple status map for monitoring
func (psm *PluginSyncManager) GetSyncStatus() (map[string]any, error) {
	local, err := psm.scanLocalPlugins()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"local_plugins_count": len(local), "local_plugins": local, "last_sync_attempt": time.Now().Format(time.RFC3339)}, nil
}
