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

// LoadConfig loads a JSON file from path into a Config. If the file does not
// exist an empty Config is returned and no error.
func LoadConfig(path string) (Config, error) {
	cfg := Config{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes the Config to path as indented JSON.
func (c Config) Save(path string) error {
	jb, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, jb, 0644)
}

// NormalizeServer normalizes the stored "server" value: ws/wss -> http/https
// and strips path/query/fragment.
func (c Config) NormalizeServer() {
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
	if strings.TrimSpace(c["bizhawk_path"]) == "" {
		c["bizhawk_path"] = filepath.Join("BizHawk-2.10-win-x64", "EmuHawk.exe")
	}
	if c["rom_dir"] == "" {
		c["rom_dir"] = "roms"
	}
	if c["save_dir"] == "" {
		c["save_dir"] = "saves"
	}
	if c["bizhawk_ipc_port"] == "" {
		c["bizhawk_ipc_port"] = "55355"
	}
	return nil
}
