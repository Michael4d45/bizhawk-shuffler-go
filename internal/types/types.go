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

func (c CommandName) String() string {
	return string(c)
}

func (g GameMode) String() string {
	return string(g)
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
	// Games is the active list of game file names to shuffle through.
	// Keep the JSON key "games" for backwards compatibility with the UI.
	Games []string `json:"games"`
	// MainGames is the main catalog of games on the server. Each entry
	// describes the primary file and any additional files that clients
	// should also download when preparing this game.
	MainGames []GameEntry       `json:"main_games,omitempty"`
	Players   map[string]Player `json:"players"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// GameEntry describes a single catalog entry in the server's main game list.
// File is the primary filename; ExtraFiles lists additional files the client
// should download when preparing the game (for example assets or patches).
type GameEntry struct {
	File       string   `json:"file"`
	ExtraFiles []string `json:"extra_files,omitempty"`
}

// Player represents a connected client
type Player struct {
	Name      string `json:"name"`
	Current   string `json:"current_game"`
	HasFiles  bool   `json:"has_files"`
	Connected bool   `json:"connected"`
	// PingMs stores the last measured round-trip time to the player in milliseconds.
	PingMs int `json:"ping_ms,omitempty"`
}
