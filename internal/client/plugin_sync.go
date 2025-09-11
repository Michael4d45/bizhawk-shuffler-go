package client

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// PluginSyncManager handles synchronization of plugins between client and server
type PluginSyncManager struct {
	api        *API
	httpClient *http.Client
	config     Config
	logger     *log.Logger
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
	logger := log.New(os.Stdout, "[PluginSync] ", log.LstdFlags)
	return &PluginSyncManager{
		api:        api,
		httpClient: httpClient,
		config:     config,
		logger:     logger,
	}
}

// SyncPlugins performs a complete plugin synchronization with the server
func (psm *PluginSyncManager) SyncPlugins() (*PluginSyncResult, error) {
	psm.logger.Println("=== Starting Plugin Synchronization ===")
	startTime := time.Now()

	result := &PluginSyncResult{
		StartTime: startTime,
		Errors:    make([]string, 0),
	}

	// Step 1: Fetch enabled plugins from server
	psm.logger.Println("Step 1: Fetching plugin list from server...")
	serverPlugins, err := psm.fetchServerPlugins()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to fetch server plugins: %v", err))
		psm.logger.Printf("ERROR: Failed to fetch server plugins: %v", err)
		return result, err
	}

	result.TotalPlugins = len(serverPlugins)
	psm.logger.Printf("Found %d plugins on server", result.TotalPlugins)

	// Step 2: Get local plugin state
	psm.logger.Println("Step 2: Scanning local plugin directory...")
	localPlugins, err := psm.scanLocalPlugins()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to scan local plugins: %v", err))
		psm.logger.Printf("ERROR: Failed to scan local plugins: %v", err)
		return result, err
	}

	psm.logger.Printf("Found %d local plugins", len(localPlugins))

	// Step 3: Determine what needs to be done
	psm.logger.Println("Step 3: Analyzing sync requirements...")
	toDownload, toUpdate, toRemove := psm.analyzeSyncRequirements(serverPlugins, localPlugins)

	psm.logger.Printf("Analysis complete: %d to download, %d to update, %d to remove",
		len(toDownload), len(toUpdate), len(toRemove))

	// Step 4: Download new plugins
	psm.logger.Println("Step 4: Downloading new plugins...")
	for _, pluginName := range toDownload {
		psm.logger.Printf("Downloading plugin: %s", pluginName)
		if err := psm.downloadPlugin(pluginName); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to download %s: %v", pluginName, err))
			psm.logger.Printf("ERROR: Failed to download %s: %v", pluginName, err)
		} else {
			result.Downloaded++
			psm.logger.Printf("SUCCESS: Downloaded %s", pluginName)
		}
	}

	// Step 5: Update existing plugins
	psm.logger.Println("Step 5: Updating existing plugins...")
	for _, pluginName := range toUpdate {
		psm.logger.Printf("Updating plugin: %s", pluginName)
		if err := psm.downloadPlugin(pluginName); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to update %s: %v", pluginName, err))
			psm.logger.Printf("ERROR: Failed to update %s: %v", pluginName, err)
		} else {
			result.Updated++
			psm.logger.Printf("SUCCESS: Updated %s", pluginName)
		}
	}

	// Step 6: Remove disabled plugins
	psm.logger.Println("Step 6: Removing disabled plugins...")
	for _, pluginName := range toRemove {
		psm.logger.Printf("Removing plugin: %s", pluginName)
		if err := psm.removeLocalPlugin(pluginName); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to remove %s: %v", pluginName, err))
			psm.logger.Printf("ERROR: Failed to remove %s: %v", pluginName, err)
		} else {
			result.Removed++
			psm.logger.Printf("SUCCESS: Removed %s", pluginName)
		}
	}

	// Step 7: Final verification
	psm.logger.Println("Step 7: Verifying sync completion...")
	finalPlugins, err := psm.scanLocalPlugins()
	if err != nil {
		psm.logger.Printf("WARNING: Could not verify final state: %v", err)
	} else {
		psm.logger.Printf("Final state: %d plugins locally installed", len(finalPlugins))
	}

	result.Duration = time.Since(startTime)
	psm.logger.Printf("=== Plugin Synchronization Complete ===")
	psm.logger.Printf("Duration: %v", result.Duration)
	psm.logger.Printf("Summary: %d total, %d downloaded, %d updated, %d removed",
		result.TotalPlugins, result.Downloaded, result.Updated, result.Removed)

	if len(result.Errors) > 0 {
		psm.logger.Printf("Errors encountered: %d", len(result.Errors))
		for _, err := range result.Errors {
			psm.logger.Printf("  - %s", err)
		}
	}

	return result, nil
}

// fetchServerPlugins retrieves the list of enabled plugins from the server
func (psm *PluginSyncManager) fetchServerPlugins() (map[string]types.Plugin, error) {
	psm.logger.Println("Fetching plugin list from server API...")

	resp, err := psm.httpClient.Get(psm.api.BaseURL + "/api/plugins")
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	psm.logger.Printf("Raw server response: %s", string(body))

	var response struct {
		Plugins map[string]types.Plugin `json:"plugins"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Filter to only enabled plugins
	enabledPlugins := make(map[string]types.Plugin)
	for name, plugin := range response.Plugins {
		if plugin.Enabled {
			enabledPlugins[name] = plugin
			psm.logger.Printf("Server has enabled plugin: %s (v%s)", name, plugin.Version)
		} else {
			psm.logger.Printf("Server has disabled plugin: %s (skipping)", name)
		}
	}

	psm.logger.Printf("Server reports %d enabled plugins", len(enabledPlugins))
	return enabledPlugins, nil
}

// scanLocalPlugins scans the local plugin directory and returns plugin metadata
func (psm *PluginSyncManager) scanLocalPlugins() (map[string]types.Plugin, error) {
	pluginDir := "./plugins"
	psm.logger.Printf("Scanning local plugin directory: %s", pluginDir)

	// Ensure directory exists
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create plugin directory: %w", err)
	}

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin directory: %w", err)
	}

	localPlugins := make(map[string]types.Plugin)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginName := entry.Name()
		psm.logger.Printf("Found local plugin directory: %s", pluginName)

		// Try to read plugin metadata
		metaPath := filepath.Join(pluginDir, pluginName, "meta.json")
		if _, err := os.Stat(metaPath); os.IsNotExist(err) {
			psm.logger.Printf("WARNING: No meta.json found for plugin %s", pluginName)
			// Create basic plugin info for plugins without metadata
			localPlugins[pluginName] = types.Plugin{
				Name:        pluginName,
				Version:     "unknown",
				Description: "Plugin without metadata",
				Enabled:     true, // Assume enabled if present
			}
			continue
		}

		metaFile, err := os.Open(metaPath)
		if err != nil {
			psm.logger.Printf("ERROR: Failed to open meta.json for %s: %v", pluginName, err)
			continue
		}

		var plugin types.Plugin
		if err := json.NewDecoder(metaFile).Decode(&plugin); err != nil {
			psm.logger.Printf("ERROR: Failed to parse meta.json for %s: %v", pluginName, err)
			if cerr := metaFile.Close(); cerr != nil {
				psm.logger.Printf("WARNING: Failed to close meta.json for %s: %v", pluginName, cerr)
			}
			continue
		}
		if cerr := metaFile.Close(); cerr != nil {
			psm.logger.Printf("WARNING: Failed to close meta.json for %s: %v", pluginName, cerr)
		}

		localPlugins[pluginName] = plugin
		psm.logger.Printf("Loaded local plugin: %s (v%s)", pluginName, plugin.Version)
	}

	psm.logger.Printf("Local scan complete: found %d plugins", len(localPlugins))
	return localPlugins, nil
}

// analyzeSyncRequirements compares server and local plugins to determine what needs to be done
func (psm *PluginSyncManager) analyzeSyncRequirements(serverPlugins, localPlugins map[string]types.Plugin) (toDownload, toUpdate, toRemove []string) {
	psm.logger.Println("Analyzing sync requirements...")

	// Find plugins to download (on server but not local)
	for name := range serverPlugins {
		if _, exists := localPlugins[name]; !exists {
			toDownload = append(toDownload, name)
			psm.logger.Printf("Plugin %s needs to be downloaded", name)
		}
	}

	// Find plugins to update (on both, but different versions)
	for name, serverPlugin := range serverPlugins {
		if localPlugin, exists := localPlugins[name]; exists {
			if serverPlugin.Version != localPlugin.Version {
				toUpdate = append(toUpdate, name)
				psm.logger.Printf("Plugin %s needs update: local v%s -> server v%s",
					name, localPlugin.Version, serverPlugin.Version)
			} else {
				psm.logger.Printf("Plugin %s is up to date (v%s)", name, serverPlugin.Version)
			}
		}
	}

	// Find plugins to remove (local but not enabled on server)
	for name := range localPlugins {
		if _, exists := serverPlugins[name]; !exists {
			toRemove = append(toRemove, name)
			psm.logger.Printf("Plugin %s needs to be removed", name)
		}
	}

	return toDownload, toUpdate, toRemove
}

// downloadPlugin downloads a plugin from the server
func (psm *PluginSyncManager) downloadPlugin(pluginName string) error {
	psm.logger.Printf("Downloading plugin: %s", pluginName)

	// Create local plugin directory
	localPluginDir := filepath.Join("./plugins", pluginName)
	if err := os.MkdirAll(localPluginDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	// Download plugin.lua
	pluginURL := fmt.Sprintf("%s/files/plugins/%s/plugin.lua", psm.api.BaseURL, pluginName)
	psm.logger.Printf("Downloading from: %s", pluginURL)

	resp, err := psm.httpClient.Get(pluginURL)
	if err != nil {
		return fmt.Errorf("failed to download plugin.lua: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d for plugin.lua", resp.StatusCode)
	}

	pluginPath := filepath.Join(localPluginDir, "plugin.lua")
	pluginFile, err := os.Create(pluginPath)
	if err != nil {
		return fmt.Errorf("failed to create plugin.lua: %w", err)
	}
	defer func() { _ = pluginFile.Close() }()

	if _, err := io.Copy(pluginFile, resp.Body); err != nil {
		return fmt.Errorf("failed to save plugin.lua: %w", err)
	}

	psm.logger.Printf("Downloaded plugin.lua to: %s", pluginPath)

	// Download meta.json
	metaURL := fmt.Sprintf("%s/files/plugins/%s/meta.json", psm.api.BaseURL, pluginName)
	psm.logger.Printf("Downloading meta.json from: %s", metaURL)

	resp, err = psm.httpClient.Get(metaURL)
	if err != nil {
		psm.logger.Printf("WARNING: Failed to download meta.json: %v", err)
		// Don't fail the whole download if meta.json is missing
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		metaPath := filepath.Join(localPluginDir, "meta.json")
		metaFile, err := os.Create(metaPath)
		if err != nil {
			psm.logger.Printf("WARNING: Failed to create meta.json: %v", err)
			return nil
		}
		defer func() { _ = metaFile.Close() }()

		if _, err := io.Copy(metaFile, resp.Body); err != nil {
			psm.logger.Printf("WARNING: Failed to save meta.json: %v", err)
			return nil
		}

		psm.logger.Printf("Downloaded meta.json to: %s", metaPath)
	} else {
		psm.logger.Printf("WARNING: Server returned status %d for meta.json", resp.StatusCode)
	}

	psm.logger.Printf("Successfully downloaded plugin: %s", pluginName)
	return nil
}

// removeLocalPlugin removes a plugin from the local filesystem
func (psm *PluginSyncManager) removeLocalPlugin(pluginName string) error {
	psm.logger.Printf("Removing local plugin: %s", pluginName)

	pluginDir := filepath.Join("./plugins", pluginName)

	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		psm.logger.Printf("Plugin directory doesn't exist: %s", pluginDir)
		return nil // Already removed
	}

	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("failed to remove plugin directory: %w", err)
	}

	psm.logger.Printf("Successfully removed plugin directory: %s", pluginDir)
	return nil
}

// GetSyncStatus returns current sync status for monitoring
func (psm *PluginSyncManager) GetSyncStatus() (map[string]interface{}, error) {
	localPlugins, err := psm.scanLocalPlugins()
	if err != nil {
		return nil, err
	}

	status := map[string]interface{}{
		"local_plugins_count": len(localPlugins),
		"local_plugins":       localPlugins,
		"last_sync_attempt":   time.Now().Format(time.RFC3339),
	}

	return status, nil
}
