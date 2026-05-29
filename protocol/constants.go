package protocol

import (
	"fmt"
	"time"
)

const (
	SwapWaitMs         = 20_000
	IPCTimeoutMs       = 10_000
	SaveReadyTimeoutMs = 30_000
	PersistDebounceMs  = 500
	DiscoveryValidMs   = 30_000
)

// ServerConfig is runtime configuration for the session server.
type ServerConfig struct {
	DataDir   string
	Host      string
	Port      int
	StaticDir string
}

// ClientConfig is runtime configuration for a player client.
type ClientConfig struct {
	DataDir    string
	ServerURL  string
	PlayerName string
}

func DefaultServerState() ServerState {
	return ServerState{
		Running:             false,
		SwapEnabled:         false,
		Mode:                GameModeSync,
		Players:             map[string]Player{},
		UpdatedAt:           time.Now(),
		PreventSameGameSwap: false,
		CountdownEnabled:    false,
		MinIntervalSecs:     5,
		MaxIntervalSecs:     10,
		ConfigKeys:          []string{"DisplayFps"},
		MainGames:           []GameEntry{},
		Games:               []string{},
		GameSwapInstances:   []GameSwapInstance{},
		Plugins:             map[string]Plugin{},
	}
}

func GetServerWsURL(host string, port int) string {
	return fmt.Sprintf("ws://%s:%d/ws", host, port)
}
