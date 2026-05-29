package clienthost

import (
	"encoding/json"
	"net/url"
	"os"
)

// Config is a string map persisted as config.json in the client data directory.
type Config map[string]string

// LoadConfig loads config.json from dataDir.
func LoadConfig(dataDir string) (Config, error) {
	cfg := Config{}
	path := configPath(dataDir)
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
	cfg.normalizeServer()
	return cfg, nil
}

// SaveConfig writes config to dataDir/config.json.
func SaveConfig(dataDir string, c Config) error {
	jb, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(configPath(dataDir), jb, 0o644)
}

// Save writes the config using data_dir from the map, or the current directory.
func (c Config) Save() error {
	if d := c["data_dir"]; d != "" {
		return SaveConfig(d, c)
	}
	return SaveConfig(".", c)
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
	if c["auto_open_bizhawk"] == "" {
		c["auto_open_bizhawk"] = "true"
	}
	return nil
}

// GetBool returns the boolean value of the given key. Defaults to false if not
// found or invalid.
func (c Config) GetBool(key string) bool {
	v, ok := c[key]
	if !ok {
		return false
	}
	return v == "true" || v == "1" || v == "yes"
}

// SetBool sets the boolean value of the given key as "true" or "false".
func (c Config) SetBool(key string, val bool) {
	if val {
		c[key] = "true"
	} else {
		c[key] = "false"
	}
}
