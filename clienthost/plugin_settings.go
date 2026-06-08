package clienthost

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	pluginSettingsIPCTimeout       = 30 * time.Second
	pluginSettingsIPCRetryInterval = 500 * time.Millisecond
)

// SavePluginSettingsToFile writes settings to ./plugins/{name}/settings.kv atomically.
func SavePluginSettingsToFile(pluginName string, settings map[string]string) error {
	pluginDir := filepath.Join("./plugins", pluginName)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("failed to create plugin dir: %w", err)
	}

	settingsKV := filepath.Join(pluginDir, "settings.kv")
	tmp := settingsKV + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, exists := settings["status"]; !exists {
		settings["status"] = "disabled"
	}

	if _, err := fmt.Fprintf(f, "status = %s\n", settings["status"]); err != nil {
		return fmt.Errorf("failed to write status: %w", err)
	}

	keys := make([]string, 0, len(settings))
	for k := range settings {
		if k != "status" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := strings.ReplaceAll(settings[k], "\n", "\\n")
		if _, err := fmt.Fprintf(f, "%s = %s\n", k, val); err != nil {
			return fmt.Errorf("failed to write setting %s: %w", k, err)
		}
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}
	if err := os.Rename(tmp, settingsKV); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}
	return nil
}

// ParsePluginSettingsPayload extracts plugin settings from a state_update command payload.
func ParsePluginSettingsPayload(payload any) (pluginName string, settings map[string]string, ok bool) {
	m, isMap := payload.(map[string]any)
	if !isMap {
		return "", nil, false
	}
	name, hasName := m["plugin_name"].(string)
	if !hasName || name == "" {
		return "", nil, false
	}
	settingsMap, hasSettings := m["settings"].(map[string]any)
	if !hasSettings {
		log.Printf("plugin settings update for %s: missing settings in payload", name)
		return "", nil, false
	}
	out := make(map[string]string, len(settingsMap))
	for k, v := range settingsMap {
		if str, isStr := v.(string); isStr {
			out[k] = str
		} else {
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return name, out, true
}

// ApplyPluginSettingsUpdate saves settings locally and notifies BizHawk via PLUGIN_SETTINGS.
// Retries while IPC is temporarily unavailable so admin changes apply during play.
func ApplyPluginSettingsUpdate(bipc *BizhawkIPC, pluginName string, settings map[string]string) error {
	if bipc == nil {
		return fmt.Errorf("bizhawk ipc not configured")
	}
	if err := SavePluginSettingsToFile(pluginName, settings); err != nil {
		return fmt.Errorf("save settings.kv: %w", err)
	}
	log.Printf("updated plugin settings file for %s", pluginName)

	deadline := time.Now().Add(pluginSettingsIPCTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		lastErr = bipc.SendPluginSettings(ctx, pluginName)
		cancel()
		if lastErr == nil {
			log.Printf("sent PLUGIN_SETTINGS to BizHawk for %s", pluginName)
			return nil
		}
		if !isPluginSettingsIPCRetryable(lastErr) {
			return fmt.Errorf("PLUGIN_SETTINGS for %s: %w", pluginName, lastErr)
		}
		time.Sleep(pluginSettingsIPCRetryInterval)
	}
	return fmt.Errorf("PLUGIN_SETTINGS for %s timed out after %s: %w", pluginName, pluginSettingsIPCTimeout, lastErr)
}

func isPluginSettingsIPCRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not connected") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "ipc disconnected")
}
