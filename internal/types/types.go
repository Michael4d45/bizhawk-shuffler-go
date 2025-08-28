package types

import (
	"encoding/json"
	"time"
)

// CommandName enumerates allowed websocket command names. Use string constants
// so code can use the literal values directly without parsing.
type CommandName string

const (
	CmdHello          CommandName = "hello"
	CmdPing           CommandName = "ping"
	CmdStart          CommandName = "start"
	CmdPause          CommandName = "pause"
	CmdSwap           CommandName = "swap"
	CmdAck            CommandName = "ack"
	CmdNack           CommandName = "nack"
	CmdStatus         CommandName = "status"
	CmdGamesUpdate    CommandName = "games_update"
	CmdGamesUpdateAck CommandName = "games_update_ack"
	CmdStateUpdate    CommandName = "state_update"
	CmdClearSaves     CommandName = "clear_saves"
	CmdReset          CommandName = "reset"
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
	MainGames []GameEntry       `json:"main_games,omitempty"`
	Players   map[string]Player `json:"players"`
	UpdatedAt time.Time         `json:"updated_at"`

	Games             []string           `json:"games,omitempty"`
	GameSwapInstances []GameSwapInstance `json:"game_instances,omitempty"`
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
}
