package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config is a simple map-backed configuration used by the client code. We use
// a defined type so we can add helper methods, and convert to
// map[string]string when calling legacy functions.
type Config map[string]string

// LoadConfig loads a JSON file from "config.json" into a Config. If the file does not
// exist an empty Config is returned and no error.
func LoadConfig() (Config, error) {
	cfg := Config{}
	b, err := os.ReadFile("config.json")
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	cfg.normalizeServer()
	return cfg, nil
}

// Save writes the Config to "config.json" as indented JSON.
func (c Config) Save() error {
	jb, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir("config.json")
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile("config.json", jb, 0644)
}

// normalizeServer normalizes the stored "server" value: ws/wss -> http/https
// and strips "config.json"/query/fragment.
func (c Config) normalizeServer() {
	if s, ok := c["server"]; ok && s != "" {
		if u, err := url.Parse(s); err == nil {
			switch u.Scheme {
			case "ws":
				u.Scheme = "http"
			case "wss":
				u.Scheme = "https"
			}
			u.Path = ""
			u.RawQuery = ""
			u.Fragment = ""
			c["server"] = u.String()
		}
	}
}

// EnsureDefaults populates default values for commonly used keys if missing.
// Returns an error when a default cannot be chosen for the current platform.
func (c Config) EnsureDefaults() error {
	if c["bizhawk_download_url"] == "" {
		switch goos := runtime.GOOS; goos {
		case "windows":
			c["bizhawk_download_url"] = "https://github.com/TASEmulators/BizHawk/releases/download/2.10/BizHawk-2.10-win-x64.zip"
		case "linux":
			c["bizhawk_download_url"] = "https://github.com/TASEmulators/BizHawk/releases/download/2.10/BizHawk-2.10-linux-x64.tar.gz"
		default:
			return fmt.Errorf("no default BizHawk download URL for OS: %s", goos)
		}
	}
	// Derive a sensible default bizhawk_path from the download URL when not
	// explicitly configured. For zip archives we assume the top-level folder
	// name matches the archive basename without extension. For tar.gz archives
	// strip the .tar.gz suffix.
	if strings.TrimSpace(c["bizhawk_path"]) == "" {
		// default to a platform-aware executable name
		dl := c["bizhawk_download_url"]
		base := filepath.Base(dl)
		installDir := strings.TrimSuffix(base, filepath.Ext(base))
		if strings.HasSuffix(strings.ToLower(base), ".tar.gz") || strings.HasSuffix(strings.ToLower(base), ".tgz") {
			// base currently trimmed by filepath.Ext removed .gz; ensure .tar is removed too
			installDir = strings.TrimSuffix(installDir, ".tar")
		}
		if runtime.GOOS == "windows" {
			c["bizhawk_path"] = filepath.Join(installDir, "EmuHawk.exe")
		} else {
			c["bizhawk_path"] = filepath.Join(installDir, "EmuHawkMono.sh")
		}
	}
	return nil
}
