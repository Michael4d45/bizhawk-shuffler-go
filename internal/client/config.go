package client

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
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
func (c Config) EnsureDefaults() error {
	// bizhawk_path should be set by installer or manually
	// Client expects BizHawk to be pre-installed
	if c["discovery_enabled"] == "" {
		c["discovery_enabled"] = "true"
	}
	if c["discovery_timeout_seconds"] == "" {
		c["discovery_timeout_seconds"] = "5"
	}
	if c["multicast_address"] == "" {
		c["multicast_address"] = "239.255.255.250:1900"
	}
	if c["broadcast_interval_seconds"] == "" {
		c["broadcast_interval_seconds"] = "5"
	}
	return nil
}
