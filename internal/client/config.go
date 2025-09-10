package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
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
	if c["discovery_enabled"] == "" {
		c["discovery_enabled"] = "true"
	}
	if c["discovery_timeout_seconds"] == "" {
		c["discovery_timeout_seconds"] = "10"
	}
	if c["multicast_address"] == "" {
		c["multicast_address"] = "239.255.255.250:1900"
	}
	if c["broadcast_interval_seconds"] == "" {
		c["broadcast_interval_seconds"] = "5"
	}
	// P2P save state config defaults
	if c["p2p_enabled"] == "" { // default disabled until maturity
		c["p2p_enabled"] = "false"
	}
	if c["save_p2p_piece_size"] == "" {
		c["save_p2p_piece_size"] = "65536" // 64KB
	}
	if c["save_p2p_timeout"] == "" {
		c["save_p2p_timeout"] = "30s"
	}
	if c["save_p2p_max_peers"] == "" {
		c["save_p2p_max_peers"] = "10"
	}
	if c["save_p2p_upload_limit"] == "" {
		c["save_p2p_upload_limit"] = "0" // 0 = unlimited
	}
	return nil
}

// P2PEnabled returns true if P2P is enabled in config.
func (c Config) P2PEnabled() bool { return strings.ToLower(c["p2p_enabled"]) == "true" }

// SaveP2PPieceSize returns validated piece size (defaults applied if invalid).
func (c Config) SaveP2PPieceSize() int {
	v := 65536
	if ps := strings.TrimSpace(c["save_p2p_piece_size"]); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil {
			if n >= 16384 && n <= 1_048_576 { // 16KB - 1MB
				v = n
			}
		}
	}
	return v
}

// SaveP2PTimeout returns duration for P2P operations.
func (c Config) SaveP2PTimeout() time.Duration {
	d := 30 * time.Second
	if ts := strings.TrimSpace(c["save_p2p_timeout"]); ts != "" {
		if dur, err := time.ParseDuration(ts); err == nil {
			if dur >= 10*time.Second && dur <= 5*time.Minute {
				d = dur
			}
		}
	}
	return d
}

// SaveP2PMaxPeers returns maximum peers per swarm.
func (c Config) SaveP2PMaxPeers() int {
	v := 10
	if mp := strings.TrimSpace(c["save_p2p_max_peers"]); mp != "" {
		if n, err := strconv.Atoi(mp); err == nil && n >= 1 && n <= 200 {
			v = n
		}
	}
	return v
}

// SaveP2PUploadLimit returns upload limit in bytes/sec (0 = unlimited).
func (c Config) SaveP2PUploadLimit() int64 {
	if ul := strings.TrimSpace(c["save_p2p_upload_limit"]); ul != "" {
		// Accept suffixes KB/MB for convenience
		lower := strings.ToLower(ul)
		mult := int64(1)
		if strings.HasSuffix(lower, "kb") {
			mult = 1024
			lower = strings.TrimSuffix(lower, "kb")
		} else if strings.HasSuffix(lower, "mb") {
			mult = 1024 * 1024
			lower = strings.TrimSuffix(lower, "mb")
		}
		lower = strings.TrimSpace(lower)
		if n, err := strconv.ParseInt(lower, 10, 64); err == nil && n >= 0 {
			return n * mult
		}
	}
	return 0
}
