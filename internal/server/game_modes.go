package server

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// GameModeHandler defines the interface for implementing game mode behavior
type GameModeHandler interface {
	// HandleSwap performs the swap operation for this game mode
	HandleSwap() error

	// GetCurrentGameForPlayer determines what game a player should be playing in this mode
	GetCurrentGameForPlayer(player string) string

	SetupState() error
	// HandlePlayerSwap updates server state for a player-level swap (assign instances, set player->game mapping, etc)
	HandlePlayerSwap(player string, game string, instanceID string) error
}

// SyncModeHandler implements the sync game mode
type SyncModeHandler struct {
	server *Server
}

func (h *SyncModeHandler) HandleSwap() error {
	game := h.randomGame()
	if game == "" {
		return errors.New("no games available for swap")
	}
	// In sync mode, set all players to the same game
	h.server.mu.Lock()
	defer h.server.mu.Unlock()
	for name, player := range h.server.state.Players {
		player.Game = game
		h.server.state.Players[name] = player
		if player.Connected {
			payload := map[string]string{"game": game}
			// Notify connected players about the game change
			h.server.sendAndWait(name, types.Command{
				Cmd:     types.CmdSwap,
				Payload: payload,
				ID:      fmt.Sprintf("swap-%d", time.Now().UnixNano()),
			}, 20*time.Second)
		}
	}
	h.server.state.UpdatedAt = time.Now()
	return nil
}

func (h *SyncModeHandler) randomGame() string {
	games := h.server.state.Games
	if len(games) == 0 {
		return ""
	}
	return games[rand.Intn(len(games))]
}

func (h *SyncModeHandler) GetCurrentGameForPlayer(player string) string {
	h.server.mu.Lock()
	defer h.server.mu.Unlock()

	for _, pp := range h.server.state.Players {
		if pp.Game != "" {
			return pp.Game
		}
	}

	if len(h.server.state.Games) > 0 {
		return h.randomGame()
	}

	return ""
}

func (h *SyncModeHandler) SetupState() error {
	if len(h.server.state.MainGames) < 2 {
		return errors.New("expected multiple games")
	}

	return nil
}

func (h *SyncModeHandler) HandlePlayerSwap(player string, game string, instanceID string) error {
	// In sync mode we don't use instances; just set the player's current game
	h.server.mu.Lock()
	defer h.server.mu.Unlock()
	p, ok := h.server.state.Players[player]
	if !ok {
		p = types.Player{Name: player}
	}
	p.Game = game
	h.server.state.Players[player] = p
	h.server.state.UpdatedAt = time.Now()
	return nil
}

// SaveModeHandler implements the save game mode
type SaveModeHandler struct {
	server *Server
}

func (h *SaveModeHandler) HandleSwap() error {
	return nil
}

func (h *SaveModeHandler) GetCurrentGameForPlayer(player string) string {
	// In save mode, return first instance game if available, else fallbacks
	if len(h.server.state.GameSwapInstances) > 0 {
		return h.server.state.GameSwapInstances[0].Game
	}
	if len(h.server.state.MainGames) > 0 {
		return h.server.state.MainGames[0].File
	}
	return ""
}

func (h *SaveModeHandler) SetupState() error {
	return nil
}

func (h *SaveModeHandler) HandlePlayerSwap(player string, game string, instanceID string) error {
	h.server.mu.Lock()
	defer h.server.mu.Unlock()

	// If instance ID provided, assign that instance to the player
	if instanceID != "" {
		for i, inst := range h.server.state.GameSwapInstances {
			if inst.ID == instanceID {
				h.server.state.GameSwapInstances[i].Player = player
				// update player entry
				p, ok := h.server.state.Players[player]
				if !ok {
					p = types.Player{Name: player}
				}
				p.Game = h.server.state.GameSwapInstances[i].Game
				p.Connected = true
				h.server.state.Players[player] = p
				h.server.state.UpdatedAt = time.Now()
				return nil
			}
		}
		return errors.New("instance not found")
	}

	// If no instance ID, try to find an unassigned instance matching the game and assign it.
	for i, inst := range h.server.state.GameSwapInstances {
		if inst.Game == game && inst.Player == "" {
			h.server.state.GameSwapInstances[i].Player = player
			p, ok := h.server.state.Players[player]
			if !ok {
				p = types.Player{Name: player}
			}
			p.Game = game
			p.Connected = true
			h.server.state.Players[player] = p
			h.server.state.UpdatedAt = time.Now()
			return nil
		}
	}

	// Fallback: set player's game without assigning an instance
	p, ok := h.server.state.Players[player]
	if !ok {
		p = types.Player{Name: player}
	}
	p.Game = game
	p.Connected = true
	h.server.state.Players[player] = p
	h.server.state.UpdatedAt = time.Now()
	return nil
}

// getGameModeHandler returns the appropriate handler for the given game mode
func (s *Server) GetGameModeHandler() GameModeHandler {
	s.mu.Lock()
	mode := s.state.Mode
	s.mu.Unlock()

	switch mode {
	case types.GameModeSync:
		return &SyncModeHandler{
			server: s,
		}
	case types.GameModeSave:
		return &SaveModeHandler{
			server: s,
		}
	default:
		panic("unexpected game mode: \"" + mode + "\"")
	}
}
