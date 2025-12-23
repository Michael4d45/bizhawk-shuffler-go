package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DiscoveryMessage represents a UDP broadcast message sent by servers
type DiscoveryMessage struct {
	Type       string    `json:"type"`        // "bizshuffle_server"
	Version    string    `json:"version"`     // Protocol version
	ServerName string    `json:"server_name"` // Human-readable server name
	Host       string    `json:"host"`        // Server host/IP
	Port       int       `json:"port"`        // Server port
	Timestamp  time.Time `json:"timestamp"`   // When message was sent
	ServerID   string    `json:"server_id"`   // Unique server identifier
}

// ServerInfo contains basic information about a discovered server
type ServerInfo struct {
	Name     string    `json:"name"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	Version  string    `json:"version"`
	LastSeen time.Time `json:"last_seen"`
	ServerID string    `json:"server_id"`
}

// DiscoveryConfig holds configuration for LAN discovery
type DiscoveryConfig struct {
	Enabled              bool   `json:"enabled"`
	MulticastAddress     string `json:"multicast_address"`      // e.g., "239.255.255.250:1900"
	BroadcastIntervalSec int    `json:"broadcast_interval_sec"` // How often to broadcast (seconds)
	ListenTimeoutSec     int    `json:"listen_timeout_sec"`     // How long to listen for broadcasts (seconds)
}

// CommandName enumerates allowed websocket command names. Use string constants
// so code can use the literal values directly without parsing.
type CommandName string

const (
	// From Client to Server
	CmdHello          CommandName = "hello"
	CmdAck            CommandName = "ack"
	CmdNack           CommandName = "nack"
	CmdGamesUpdateAck CommandName = "games_update_ack"
	CmdStatusUpdate   CommandName = "status_update"
	CmdTypeLua        CommandName = "lua_command"
	CmdConfigResponse CommandName = "config_response"

	// From Server to Client
	CmdPing             CommandName = "ping"
	CmdResume           CommandName = "start"
	CmdPause            CommandName = "pause"
	CmdSwap             CommandName = "swap"
	CmdMessage          CommandName = "message"
	CmdGamesUpdate      CommandName = "games_update"
	CmdClearSaves       CommandName = "clear_saves"
	CmdRequestSave      CommandName = "request_save"
	CmdPluginReload     CommandName = "plugin_reload"
	CmdFullscreenToggle CommandName = "fullscreen_toggle"
	CmdCheckConfig      CommandName = "check_config"
	CmdUpdateConfig     CommandName = "update_config"

	// From Admin to Server
	CmdHelloAdmin CommandName = "hello_admin"

	// From Server to Admin
	CmdStateUpdate CommandName = "state_update"
)

type LuaCmd string

const (
	LuaCmdSwap    LuaCmd = "swap"
	LuaCmdSwapMe  LuaCmd = "swap_me"
	LuaCmdMessage LuaCmd = "message"
)

// GameMode enumerates the available game swapping modes. Use string constants
// so callers can use the literal values directly.
type GameMode string

const (
	// GameModeSync - all players play the same game and swap simultaneously (no saves uploaded/downloaded)
	GameModeSync GameMode = "sync"
	// GameModeSave - players play different games and perform save upload/download orchestration on swap
	GameModeSave GameMode = "save"
)

// FileState tracks the state of save files for instances
type FileState string

const (
	// FileStateNone - no save file exists
	FileStateNone FileState = "none"
	// FileStatePending - save file operation is in progress
	FileStatePending FileState = "pending"
	// FileStateReady - save file is available and ready
	FileStateReady FileState = "ready"
)

func (c CommandName) String() string {
	return string(c)
}

func (g GameMode) String() string {
	return string(g)
}

func (f FileState) String() string {
	return string(f)
}

func (ps PluginStatus) String() string {
	return string(ps)
}

func (c CommandName) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(c))
}

func (c *CommandName) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*c = CommandName(s)
	return nil
}

func (g GameMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(g))
}

func (g *GameMode) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*g = GameMode(s)
	return nil
}

func (f FileState) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(f))
}

func (f *FileState) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*f = FileState(s)
	return nil
}

func (ps PluginStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(ps))
}

func (ps *PluginStatus) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*ps = PluginStatus(s)
	return nil
}

// Command is the common websocket message envelope
type Command struct {
	Cmd     CommandName `json:"cmd"`
	Payload any         `json:"payload,omitempty"`
	ID      string      `json:"id"`
}

// ServerState is persisted on the server
type ServerState struct {
	Running     bool `json:"running"`
	SwapEnabled bool `json:"swap_enabled"`
	// Mode controls the high-level server swap behavior.
	Mode GameMode `json:"mode,omitempty"`
	// Host is an optional persisted listen host (e.g. "0.0.0.0" or "127.0.0.1").
	// If present, the server can use this value when a --host flag isn't
	// provided on the command line.
	Host string `json:"host,omitempty"`
	// Port is an optional persisted listen port (e.g. 8080).
	// If present, the server can use this value when a --port flag isn't
	// provided on the command line.
	Port int `json:"port,omitempty"`
	// NextSwapAt is the unix epoch seconds when the next scheduled swap will occur.
	// It is updated by the server scheduler and persisted so the UI can display it.
	NextSwapAt      int64 `json:"next_swap_at,omitempty"`
	MinIntervalSecs int   `json:"min_interval_secs,omitempty"`
	MaxIntervalSecs int   `json:"max_interval_secs,omitempty"`
	// MainGames is the main catalog of games on the server. Each entry
	// describes the primary file and any additional files that clients
	// should also download when preparing this game.
	MainGames []GameEntry `json:"main_games,omitempty"`
	// Plugins contains the current plugin configuration and status
	Plugins   map[string]Plugin `json:"plugins,omitempty"`
	Players   map[string]Player `json:"players"`
	UpdatedAt time.Time         `json:"updated_at"`

	Games             []string           `json:"games,omitempty"`
	GameSwapInstances []GameSwapInstance `json:"game_instances,omitempty"`
	// PreventSameGameSwap prevents players from being swapped to the same game they're currently playing
	PreventSameGameSwap bool `json:"prevent_same_game_swap"`
	// CountdownEnabled enables a 3-2-1 countdown before auto swaps
	CountdownEnabled bool `json:"countdown_enabled"`
	// SwapSeed is used for deterministic random game selection in sync mode
	SwapSeed int64 `json:"swap_seed,omitempty"`
	// ConfigKeys defines the BizHawk config keys that can be managed via the UI
	ConfigKeys []string `json:"config_keys,omitempty"`
}

// GameEntry describes a single catalog entry in the server's main game list.
// File is the primary filename; ExtraFiles lists additional files that clients
// should also download when preparing this game (for example assets or patches).
type GameEntry struct {
	File       string   `json:"file"`
	ExtraFiles []string `json:"extra_files,omitempty"`
}

// Player represents a connected client
type Player struct {
	Name         string `json:"name"`
	HasFiles     bool   `json:"has_files"`
	Connected    bool   `json:"connected"`
	BizhawkReady bool   `json:"bizhawk_ready"`
	Game         string `json:"game,omitempty"`
	InstanceID   string `json:"instance_id,omitempty"`
	// PingMs stores the last measured round-trip time to the player in milliseconds.
	PingMs int `json:"ping_ms,omitempty"`
	// CompletedGames lists game files that this player has completed (for sync mode)
	CompletedGames []string `json:"completed_games,omitempty"`
	// CompletedInstances lists instance IDs that this player has completed (for save mode)
	CompletedInstances []string `json:"completed_instances,omitempty"`
	// ConfigValues stores the player's BizHawk config values for managed keys
	ConfigValues map[string]any `json:"config_values,omitempty"`
}

type GameSwapInstance struct {
	ID            string    `json:"id"`
	Game          string    `json:"game"`
	FileState     FileState `json:"file_state"`
	PendingPlayer string    `json:"pending_player,omitempty"`
}

// Plugin represents a Lua plugin that can be loaded into BizHawk
type Plugin struct {
	Name           string                 `json:"name"`
	Version        string                 `json:"version"`
	BizHawkVersion string                 `json:"bizhawk_version"`
	Description    string                 `json:"description"`
	Author         string                 `json:"author"`
	Status         PluginStatus           `json:"status"`
	SettingsMeta   map[string]SettingMeta `json:"settings_meta,omitempty"`
}

// SettingMeta defines metadata for a plugin setting to guide UI rendering
type SettingMeta struct {
	Type    string   `json:"type"`    // e.g., "dropdown", "text", "number"
	Options []string `json:"options"` // Options for dropdown type
}

// PluginStatus represents the current status of a plugin
type PluginStatus string

const (
	PluginStatusDisabled PluginStatus = "disabled"
	PluginStatusEnabled  PluginStatus = "enabled"
	PluginStatusLoading  PluginStatus = "loading"
	PluginStatusError    PluginStatus = "error"
)

// IsExpired checks if the server info is older than the given duration
func (s *ServerInfo) IsExpired(maxAge time.Duration) bool {
	return time.Since(s.LastSeen) > maxAge
}

// GetServerURL returns the WebSocket URL for this server
func (s *ServerInfo) GetServerURL() string {
	return fmt.Sprintf("ws://%s:%d/ws", s.Host, s.Port)
}

// NewDiscoveryMessage creates a new discovery message
func NewDiscoveryMessage(host string, port int, serverName string) *DiscoveryMessage {
	return &DiscoveryMessage{
		Type:       "bizshuffle_server",
		Version:    "1.0",
		ServerName: serverName,
		Host:       host,
		Port:       port,
		Timestamp:  time.Now(),
		ServerID:   fmt.Sprintf("%s:%d", host, port), // Simple ID based on host:port
	}
}

// IsValid checks if the discovery message is valid and recent
func (d *DiscoveryMessage) IsValid() bool {
	// Check if message is recent (within last 30 seconds)
	if time.Since(d.Timestamp) > 30*time.Second {
		return false
	}
	// Check required fields
	if d.Type != "bizshuffle_server" || d.Host == "" || d.Port <= 0 {
		return false
	}
	return true
}

// GetDefaultDiscoveryConfig returns default discovery configuration
func GetDefaultDiscoveryConfig() *DiscoveryConfig {
	return &DiscoveryConfig{
		Enabled:              true,
		MulticastAddress:     "239.255.255.250:1900",
		BroadcastIntervalSec: 5,
		ListenTimeoutSec:     10,
	}
}

// LuaCommand represents a parsed Lua-originated command line of the form:
//
//	CMD|kind|key=value|...
//
// Notes:
//   - Outgoing commands from Go to Lua are formatted as CMD|id|..., but incoming
//     from Lua may omit id (e.g., CMD|message|message=...)
//   - Remaining tokens after kind are treated as key=value pairs when they
//     contain '='
type LuaCommand struct {
	Raw    string            // full raw line
	Kind   LuaCmd            // lowercased command kind (e.g., "message")
	Fields map[string]string // parsed key=value fields
}

// ParseLuaCommand parses a Lua command line. It is tolerant of optional ID and
// supports both the new escaped semicolon payload and legacy pipe-delimited tokens.
func ParseLuaCommand(line string) (*LuaCommand, error) {
	s := strings.TrimSpace(line)
	if s == "" {
		return nil, fmt.Errorf("empty line")
	}
	// Split top-level by '|' but ignore escaped pipes (\|)
	parts := splitUnescaped(s, '|')
	if len(parts) < 2 || parts[0] != "CMD" {
		return nil, fmt.Errorf("invalid CMD format: %q", line)
	}

	cmd := &LuaCommand{Raw: line, Fields: make(map[string]string)}
	i := 1
	if i >= len(parts) {
		return nil, fmt.Errorf("missing kind in CMD: %q", line)
	}
	cmd.Kind = LuaCmd(strings.ToLower(strings.TrimSpace(parts[i])))
	i++

	// New format from Lua: exactly one payload segment with escaped k=v pairs separated by ';'
	if i == len(parts)-1 {
		payload := parts[i]
		if payload != "" {
			for _, tok := range splitUnescaped(payload, ';') {
				if tok == "" {
					continue
				}
				// split on first unescaped '='
				kv := splitUnescaped(tok, '=')
				if len(kv) >= 2 {
					k := strings.TrimSpace(unescapeEscaped(kv[0]))
					v := strings.TrimSpace(unescapeEscaped(strings.Join(kv[1:], "=")))
					if k != "" {
						cmd.Fields[k] = v
					}
				}
			}
		}
	} else if i < len(parts) {
		// Legacy: additional pipe-delimited tokens
		for ; i < len(parts); i++ {
			tok := strings.TrimSpace(parts[i])
			if tok == "" {
				continue
			}
			if kv := strings.SplitN(tok, "=", 2); len(kv) == 2 {
				k := strings.TrimSpace(kv[0])
				v := strings.TrimSpace(kv[1])
				cmd.Fields[k] = v
			}
		}
	}

	return cmd, nil
}

// splitUnescaped splits s by sep where sep is not escaped by a preceding backslash.
// It preserves empty segments.
func splitUnescaped(s string, sep byte) []string {
	var parts []string
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			if i == 0 || s[i-1] != '\\' {
				parts = append(parts, s[last:i])
				last = i + 1
			}
		}
	}
	parts = append(parts, s[last:])
	return parts
}

// unescapeEscaped converts Lua-side escapes (\\, \|, \;, \=) back to their literal forms.
func unescapeEscaped(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			if i+1 < len(s) {
				b.WriteByte(s[i+1])
				i++
				continue
			}
			// trailing backslash - keep it
			b.WriteByte(s[i])
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
