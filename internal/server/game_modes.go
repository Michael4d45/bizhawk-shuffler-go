package server

import (
	"fmt"
	"hash/fnv"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// GameModeHandler defines the interface for implementing game mode behavior
type GameModeHandler interface {
	// HandleSwap performs the swap operation for this game mode
	HandleSwap(s *Server) (*SwapOutcome, error)
	
	// GetCurrentGameForPlayer determines what game a player should be playing in this mode
	GetCurrentGameForPlayer(s *Server, player string) string
	
	// Description returns a human-readable description of this game mode
	Description() string
}

// SyncModeHandler implements the sync game mode
type SyncModeHandler struct{}

func (h *SyncModeHandler) HandleSwap(s *Server) (*SwapOutcome, error) {
	return s.performSwapSync()
}

func (h *SyncModeHandler) GetCurrentGameForPlayer(s *Server, player string) string {
	// In sync mode, try to find a single game that everyone should be playing
	// Look for any player with a Current set and prefer that; otherwise, fall back to first game
	for _, pl := range s.state.Players {
		if pl.Current != "" {
			return pl.Current
		}
	}
	if len(s.state.Games) > 0 {
		return s.state.Games[0]
	}
	return ""
}

func (h *SyncModeHandler) Description() string {
	return "Sync Mode: All players play the same game and swap simultaneously. No save files are uploaded or downloaded during swaps."
}

// SaveModeHandler implements the save game mode
type SaveModeHandler struct{}

func (h *SaveModeHandler) HandleSwap(s *Server) (*SwapOutcome, error) {
	return s.performSwapSave()
}

func (h *SaveModeHandler) GetCurrentGameForPlayer(s *Server, player string) string {
	// In save mode, if player's current is not set, pick a deterministic different game
	if len(s.state.Games) > 0 {
		// Use hash of player name for stable assignment
		h := fnv.New32a()
		_, _ = h.Write([]byte(player))
		idx := int(h.Sum32()) % len(s.state.Games)
		return s.state.Games[idx]
	}
	return ""
}

func (h *SaveModeHandler) Description() string {
	return "Save Mode: Players play different games and perform save upload/download orchestration on swap. Each player gets a different game assigned based on a hash of their name."
}

// getGameModeHandler returns the appropriate handler for the given game mode
func getGameModeHandler(mode types.GameMode) (GameModeHandler, error) {
	switch mode {
	case types.GameModeSync:
		return &SyncModeHandler{}, nil
	case types.GameModeSave:
		return &SaveModeHandler{}, nil
	default:
		return nil, fmt.Errorf("unknown game mode: %s", mode.String())
	}
}

// GetAllGameModes returns information about all available game modes
func GetAllGameModes() map[types.GameMode]string {
	return map[types.GameMode]string{
		types.GameModeSync: (&SyncModeHandler{}).Description(),
		types.GameModeSave: (&SaveModeHandler{}).Description(),
	}
}