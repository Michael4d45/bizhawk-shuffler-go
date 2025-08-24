package types

import (
	"encoding/json"
	"strings"
	"time"
)

// CommandName enumerates allowed websocket command names.
type CommandName int

const (
	CmdUnknown CommandName = iota
	CmdHello
	CmdStart
	CmdPause
	CmdResume
	CmdSwap
	CmdDownloadSave
	CmdAck
	CmdNack
	CmdStatus
	CmdGamesUpdate
	CmdGamesUpdateAck
	CmdStateUpdate
	CmdClearSaves
	CmdReset
	CmdToggleSwaps
)

func (c CommandName) String() string {
	switch c {
	case CmdHello:
		return "hello"
	case CmdStart:
		return "start"
	case CmdPause:
		return "pause"
	case CmdResume:
		return "resume"
	case CmdSwap:
		return "swap"
	case CmdDownloadSave:
		return "download_save"
	case CmdAck:
		return "ack"
	case CmdNack:
		return "nack"
	case CmdStatus:
		return "status"
	case CmdGamesUpdate:
		return "games_update"
	case CmdGamesUpdateAck:
		return "games_update_ack"
	case CmdStateUpdate:
		return "state_update"
	case CmdClearSaves:
		return "clear_saves"
	case CmdReset:
		return "reset"
	case CmdToggleSwaps:
		return "toggle_swaps"
	default:
		return "unknown"
	}
}

// ParseCommandName converts a string to the CommandName enum (case-insensitive).
func ParseCommandName(s string) CommandName {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "hello":
		return CmdHello
	case "start":
		return CmdStart
	case "pause":
		return CmdPause
	case "resume":
		return CmdResume
	case "swap":
		return CmdSwap
	case "download_save":
		return CmdDownloadSave
	case "ack":
		return CmdAck
	case "nack":
		return CmdNack
	case "status":
		return CmdStatus
	case "games_update":
		return CmdGamesUpdate
	case "games_update_ack":
		return CmdGamesUpdateAck
	case "state_update":
		return CmdStateUpdate
	case "clear_saves":
		return CmdClearSaves
	case "reset":
		return CmdReset
	case "toggle_swaps":
		return CmdToggleSwaps
	default:
		return CmdUnknown
	}
}

// MarshalJSON encodes the enum as a JSON string (the wire format expects a string)
func (c CommandName) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

// UnmarshalJSON parses a JSON string into the enum.
func (c *CommandName) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		// if not a string, try integer
		var iv int
		if err2 := json.Unmarshal(b, &iv); err2 == nil {
			*c = CommandName(iv)
			return nil
		}
		return err
	}
	*c = ParseCommandName(s)
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
	// Mode controls the high-level server swap behavior. Allowed values:
	// "sync"  - all players play the same game and swap simultaneously (no saves uploaded/downloaded)
	// "save"  - players play different games and perform save upload/download orchestration on swap
	Mode string `json:"mode,omitempty"`
	// Host is an optional persisted listen host (e.g. "0.0.0.0" or "127.0.0.1").
	// If present, the server can use this value when a --host flag isn't
	// provided on the command line.
	Host string `json:"host,omitempty"`
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
	// Orchestrations stores recent swap orchestration runs so an admin can
	// inspect or resume partial/failed swaps. Keyed by orchestration ID.
	Orchestrations map[string]SwapOrchestrationState `json:"orchestrations,omitempty"`
	UpdatedAt      time.Time                         `json:"updated_at"`
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
}

// SaveMetadata describes a persisted save file. This mirrors the entries
// produced by the server's ./saves/index.json and documents the canonical
// save filename convention: saves are stored under ./saves/<player>/<file>
// and a common convention is to use <game>.state as the filename for a
// game's save state on upload/download.
type SaveMetadata struct {
	Player  string `json:"player"`
	File    string `json:"file"`
	Size    int64  `json:"size"`
	At      int64  `json:"at"` // unix seconds
	Game    string `json:"game,omitempty"`
	Mime    string `json:"mime,omitempty"`
	ModTime string `json:"modtime,omitempty"`
	URL     string `json:"url,omitempty"`
}

// SwapOrchestrationState captures the state of a single swap orchestration
// run. It is persisted into ServerState.Orchestrations to allow resumption
// or inspection after partial failures.
type SwapOrchestrationState struct {
	ID          string            `json:"id"`
	Mapping     map[string]string `json:"mapping"`                // player -> game
	PrevMapping map[string]string `json:"prev_mapping,omitempty"` // previous player->game mapping for rollback
	Status      map[string]string `json:"status,omitempty"`       // per-player step status (e.g. pending/acked/verified/failed)
	Results     map[string]string `json:"results,omitempty"`      // per-player final result strings
	StartedAt   time.Time         `json:"started_at"`
	Step        string            `json:"step,omitempty"` // high level step name
	Completed   bool              `json:"completed"`
	Error       string            `json:"error,omitempty"`
}
