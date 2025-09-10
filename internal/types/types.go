package types

import (
	"encoding/json"
	"fmt"
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
	CmdHello          CommandName = "hello"
	CmdPing           CommandName = "ping"
	CmdStart          CommandName = "start"
	CmdPause          CommandName = "pause"
	CmdSwap           CommandName = "swap"
	CmdMessage        CommandName = "message"
	CmdAck            CommandName = "ack"
	CmdNack           CommandName = "nack"
	CmdStatus         CommandName = "status"
	CmdGamesUpdate    CommandName = "games_update"
	CmdGamesUpdateAck CommandName = "games_update_ack"
	CmdStateUpdate    CommandName = "state_update"
	CmdClearSaves     CommandName = "clear_saves"
	CmdReset          CommandName = "reset"
	// P2P-specific websocket commands (initial implementation scope)
	CmdP2PManifestUpdate   CommandName = "p2p_manifest_update"
	CmdP2PSaveStateRequest CommandName = "p2p_save_state_request"
	CmdP2PSaveStateStatus  CommandName = "p2p_save_state_status"
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

	// --- P2P Save State Configuration & Manifest Metadata ---
	P2PEnabled bool `json:"p2p_enabled,omitempty"`
	// Preferred piece size clients should use for P2P chunking
	SaveStatePieceSize int `json:"save_state_piece_size,omitempty"`
	// Timeout in seconds clients should wait before falling back to HTTP
	SaveStateP2PTimeoutSec int `json:"save_state_p2p_timeout_sec,omitempty"`
	// Monotonic manifest version increments when any save version changes
	SaveStateManifestVersion int64 `json:"save_state_manifest_version,omitempty"`
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
	Name       string `json:"name"`
	HasFiles   bool   `json:"has_files"`
	Connected  bool   `json:"connected"`
	Game       string `json:"game,omitempty"`
	InstanceID string `json:"instance_id,omitempty"`
	// PingMs stores the last measured round-trip time to the player in milliseconds.
	PingMs int `json:"ping_ms,omitempty"`
}

type GameSwapInstance struct {
	ID        string    `json:"id"`
	Game      string    `json:"game"`
	FileState FileState `json:"file_state"`
	// Version metadata for current authoritative save state (if any)
	SaveHash     string    `json:"save_hash,omitempty"`
	SaveSize     int64     `json:"save_size,omitempty"`
	SaveUpdated  time.Time `json:"save_updated,omitempty"`
	SavePieceLen int       `json:"save_piece_len,omitempty"`
}

// Plugin represents a Lua plugin that can be loaded into BizHawk
type Plugin struct {
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Description  string         `json:"description"`
	Author       string         `json:"author"`
	Enabled      bool           `json:"enabled"`
	EntryPoint   string         `json:"entry_point"`
	Dependencies []string       `json:"dependencies,omitempty"`
	Config       map[string]any `json:"config,omitempty"`
	Status       PluginStatus   `json:"status"`
	Path         string         `json:"path"`
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

// P2P Save State Types & Interfaces
// This section defines stable public structs used by both server and client to
// coordinate save state versioning and (future) peer-to-peer distribution.
// Remaining work expected elsewhere (not here):
//   - Implement server peer tracker (announce + peer listing)
//   - Implement client SaveStateFetcher P2P variant
//   - Wire broadcast of CmdP2PManifestUpdate on manifest changes
//   - Add piece-level hashing (optional future optimization)

// SaveStateVersion holds version metadata for a save state.
type SaveStateVersion struct {
	InstanceID string    `json:"instance_id"`
	Hash       string    `json:"hash"`
	Size       int64     `json:"size"`
	UpdatedAt  time.Time `json:"updated_at"`
	PieceSize  int       `json:"piece_size,omitempty"`
}

// SaveStateManifest lists all current save state versions.
type SaveStateManifest struct {
	Version   int64              `json:"version"`
	Generated time.Time          `json:"generated"`
	Saves     []SaveStateVersion `json:"saves"`
	PieceSize int                `json:"piece_size"`
}

// PeerInfo describes a peer advertising save states.
type PeerInfo struct {
	ID        string   `json:"id"`
	Addr      string   `json:"addr"`
	Instances []string `json:"instances"`
	LastSeen  int64    `json:"last_seen"`
}

// P2PSaveStateConfig provides client-side tuning.
type P2PSaveStateConfig struct {
	Enabled        bool          `json:"enabled"`
	PieceSize      int           `json:"piece_size"`
	Timeout        time.Duration `json:"timeout"`
	MaxPeers       int           `json:"max_peers"`
	UploadLimitBps int64         `json:"upload_limit_bps"`
}

// SaveStateFetcher abstracts retrieval and upload of save states.
type SaveStateFetcher interface {
	EnsureSaveState(instanceID string) error
	UploadSaveState(instanceID string) error
	ValidateSaveStateVersion(instanceID string) (bool, error)
	GetSaveStateVersion(instanceID string) (SaveStateVersion, error)
}
