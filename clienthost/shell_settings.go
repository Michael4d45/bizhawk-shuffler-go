package clienthost

import (
	"strconv"
	"strings"
)

// ShellSettings are desktop shell form fields persisted in config.json.
type ShellSettings struct {
	BindHost   string
	HostPort   int
	ServerURL  string
	PlayerName string
}

// DefaultShellSettings returns defaults for a new shell (hostPort 0 = pick a free port).
func DefaultShellSettings() ShellSettings {
	return ShellSettings{
		BindHost:   "127.0.0.1",
		HostPort:   0,
		ServerURL:  "http://127.0.0.1:8080",
		PlayerName: "",
	}
}

func normalizeHostPort(value int, fallback int) int {
	if value < 0 || value > 65535 {
		return fallback
	}
	return value
}

func mergeShellSettings(partial ShellSettings) ShellSettings {
	def := DefaultShellSettings()
	bindHost := strings.TrimSpace(partial.BindHost)
	if bindHost == "" {
		bindHost = def.BindHost
	}
	serverURL := strings.TrimSpace(partial.ServerURL)
	if serverURL == "" {
		serverURL = def.ServerURL
	}
	return ShellSettings{
		BindHost:   bindHost,
		HostPort:   normalizeHostPort(partial.HostPort, def.HostPort),
		ServerURL:  serverURL,
		PlayerName: strings.TrimSpace(partial.PlayerName),
	}
}

func shellFromConfig(cfg Config) ShellSettings {
	def := DefaultShellSettings()
	bindHost := strings.TrimSpace(cfg["bind_host"])
	if bindHost == "" {
		bindHost = def.BindHost
	}
	serverURL := strings.TrimSpace(cfg["server"])
	if serverURL == "" {
		serverURL = def.ServerURL
	}
	hostPort := def.HostPort
	if p := strings.TrimSpace(cfg["host_port"]); p != "" {
		if port, err := strconv.Atoi(p); err == nil {
			hostPort = port
		}
	}
	return mergeShellSettings(ShellSettings{
		BindHost:   bindHost,
		HostPort:   hostPort,
		ServerURL:  serverURL,
		PlayerName: strings.TrimSpace(cfg["name"]),
	})
}

func applyShellToConfig(cfg Config, settings ShellSettings) {
	cfg["bind_host"] = settings.BindHost
	cfg["host_port"] = strconv.Itoa(settings.HostPort)
	cfg["server"] = settings.ServerURL
	cfg["name"] = settings.PlayerName
}

// LoadShellSettings reads shell fields from config.json or returns defaults.
func LoadShellSettings(dataDir string) ShellSettings {
	cfg, err := LoadConfig(dataDir)
	if err != nil {
		return DefaultShellSettings()
	}
	return shellFromConfig(cfg)
}

// SaveShellSettings merges patch into current settings (empty bindHost/serverUrl keep current values).
func SaveShellSettings(dataDir string, patch ShellSettings) ShellSettings {
	cfg, err := LoadConfig(dataDir)
	if err != nil {
		return DefaultShellSettings()
	}
	current := shellFromConfig(cfg)
	next := current
	if strings.TrimSpace(patch.BindHost) != "" {
		next.BindHost = strings.TrimSpace(patch.BindHost)
	}
	if strings.TrimSpace(patch.ServerURL) != "" {
		next.ServerURL = strings.TrimSpace(patch.ServerURL)
	}
	if patch.PlayerName != "" {
		next.PlayerName = strings.TrimSpace(patch.PlayerName)
	}
	next = mergeShellSettings(next)
	applyShellToConfig(cfg, next)
	_ = SaveConfig(dataDir, cfg)
	return next
}

// SaveShellSettingsForm persists the full shell form (used by debounced UI save).
func SaveShellSettingsForm(dataDir, bindHost, serverURL, playerName string, hostPort int) ShellSettings {
	cfg, err := LoadConfig(dataDir)
	if err != nil {
		cfg = Config{}
	}
	next := mergeShellSettings(ShellSettings{
		BindHost:   bindHost,
		HostPort:   hostPort,
		ServerURL:  serverURL,
		PlayerName: playerName,
	})
	applyShellToConfig(cfg, next)
	_ = SaveConfig(dataDir, cfg)
	return next
}
