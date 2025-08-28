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
			if _, err := h.server.sendAndWait(name, types.Command{
				Cmd:     types.CmdSwap,
				Payload: payload,
				ID:      fmt.Sprintf("swap-%d", time.Now().UnixNano()),
			}, 20*time.Second); err != nil {
				if errors.Is(err, ErrTimeout) {
					fmt.Printf("swap timeout for %s\n", name)
				} else {
					fmt.Printf("sendAndWait error: %v\n", err)
				}
			}
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
	p, ok := h.server.state.Players[player]
	if !ok {
		p = types.Player{Name: player}
	}
	p.Game = game
	h.server.state.Players[player] = p
	h.server.state.UpdatedAt = time.Now()
	h.server.mu.Unlock()

	_ = h.server.sendSwap(player, p.Game, p.InstanceID)
	return nil
}

// SaveModeHandler implements the save game mode
type SaveModeHandler struct {
	server *Server
}

func (h *SaveModeHandler) HandleSwap() error {
	// Ensure there are instances to swap between
	if len(h.server.state.GameSwapInstances) == 0 {
		return errors.New("no game instances available for swap")
	}

	// Collect player names (preserve current map snapshot)
	h.server.mu.Lock()
	players := []string{}
	for name := range h.server.state.Players {
		players = append(players, name)
	}

	// Shuffle instances to vary assignment each swap
	rand.Shuffle(len(h.server.state.GameSwapInstances), func(i, j int) {
		h.server.state.GameSwapInstances[i], h.server.state.GameSwapInstances[j] = h.server.state.GameSwapInstances[j], h.server.state.GameSwapInstances[i]
	})

	// Clear players' InstanceID for a fresh assignment
	for n, p := range h.server.state.Players {
		p.InstanceID = ""
		p.Game = ""
		h.server.state.Players[n] = p
	}

	// Assign instances to players: one instance per player up to available instances
	maxAssign := min(len(players), len(h.server.state.GameSwapInstances))
	for i := range maxAssign {
		pname := players[i]
		inst := h.server.state.GameSwapInstances[i]
		p, ok := h.server.state.Players[pname]
		if !ok {
			p = types.Player{Name: pname}
		}
		// set player's game to the instance's game and record the instance id
		p.Game = inst.Game
		p.InstanceID = inst.ID
		// preserve existing Connected flag
		h.server.state.Players[pname] = p
	}

	// Update timestamp before sending commands
	h.server.state.UpdatedAt = time.Now()
	// capture local copy of instances for sending without holding lock while network operations run
	instances := append([]types.GameSwapInstance{}, h.server.state.GameSwapInstances...)
	playersMap := map[string]types.Player{}
	for n, p := range h.server.state.Players {
		playersMap[n] = p
	}
	h.server.mu.Unlock()
	_ = h.server.saveState()

	// Send swap command to each connected player. Include instance_id when player has an assigned instance.
	for name, p := range playersMap {
		if !p.Connected {
			continue
		}
		// find instance by player's InstanceID (players now store instance ids)
		var inst *types.GameSwapInstance
		if p.InstanceID != "" {
			for i := range instances {
				if instances[i].ID == p.InstanceID {
					inst = &instances[i]
					break
				}
			}
		}
		if inst != nil && inst.Game != "" {
			_ = h.server.sendSwap(name, p.Game, p.InstanceID)
		}
	}
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

	// If instance ID provided, assign that instance to the player (players now store InstanceID)
	if instanceID == "" {
		h.server.mu.Unlock()
		return errors.New("instance ID is required")
	}
	var foundInst *types.GameSwapInstance
	for i, inst := range h.server.state.GameSwapInstances {
		if inst.ID == instanceID {
			// capture instance
			foundInst = &h.server.state.GameSwapInstances[i]
			break
		}
	}
	if foundInst == nil {
		return errors.New("instance not found")
	}
	var foundPlayer string
	for playerName, player := range h.server.state.Players {
		if player.InstanceID == foundInst.ID {
			// Clear previous assignment
			player.Game = ""
			player.InstanceID = ""
			h.server.state.Players[playerName] = player
			foundPlayer = playerName
			break
		}
	}
	// update player entry
	p, ok := h.server.state.Players[player]
	if !ok {
		p = types.Player{Name: player}
	}
	p.Game = foundInst.Game
	p.InstanceID = foundInst.ID
	h.server.state.Players[player] = p
	h.server.state.UpdatedAt = time.Now()
	h.server.mu.Unlock()

	_ = h.server.saveState()

	h.server.sendSwap(player, foundInst.Game, foundInst.ID)
	if foundPlayer != "" {
		h.server.sendSwap(foundPlayer, "", "")
	}
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
