package server

import (
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// GameModeHandler defines the interface for implementing game mode behavior
type GameModeHandler interface {
	// HandleSwap performs the swap operation for this game mode
	HandleSwap() error

	// GetPlayer determines what game a player should be playing in this mode
	GetPlayer(player string) types.Player

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
	// Update state under write lock, but minimize critical section
	connectedPlayers := []string{}
	h.server.withLock(func() {
		for name, player := range h.server.state.Players {
			player.Game = game
			h.server.state.Players[name] = player
		}
		h.server.state.UpdatedAt = time.Now()
		for name, player := range h.server.state.Players {
			if player.Connected {
				connectedPlayers = append(connectedPlayers, name)
			}
		}
	})
	// Second loop: Execute notifications for connected players
	for _, name := range connectedPlayers {
		h.server.sendSwap(name, game, "")
	}
	return nil
}

func (h *SyncModeHandler) randomGame() string {
	var games []string
	h.server.withRLock(func() { games = h.server.state.Games })
	if len(games) == 0 {
		return ""
	}
	return games[rand.Intn(len(games))]
}

func (h *SyncModeHandler) GetPlayer(player string) types.Player {
	var result types.Player
	h.server.withRLock(func() {
		// If any player already has a game assigned, return that game for the requesting player.
		for _, pp := range h.server.state.Players {
			if pp.Game != "" {
				result = types.Player{Name: player, Game: pp.Game, InstanceID: pp.InstanceID}
				return
			}
		}
		// Otherwise pick a random game from the available games
		if len(h.server.state.Games) > 0 {
			result = types.Player{Name: player, Game: h.randomGame()}
			return
		}
		result = types.Player{Name: player}
	})
	return result
}

func (h *SyncModeHandler) SetupState() error {
	if len(h.server.state.MainGames) < 2 {
		return errors.New("expected multiple games")
	}

	return nil
}

func (h *SyncModeHandler) HandlePlayerSwap(player string, game string, instanceID string) error {
	// In sync mode we don't use instances; just set the player's current game
	h.server.withLock(func() {
		p, ok := h.server.state.Players[player]
		if !ok {
			p = types.Player{Name: player}
		}
		p.Game = game
		h.server.state.Players[player] = p
		h.server.state.UpdatedAt = time.Now()
	})

	h.server.sendSwap(player, game, instanceID)
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

	// Collect player names and perform state updates under lock where necessary
	var players []string
	h.server.withLock(func() {
		for name := range h.server.state.Players {
			players = append(players, name)
		}
		// Shuffle instances to vary assignment each swap
		rand.Shuffle(len(h.server.state.GameSwapInstances), func(i, j int) {
			h.server.state.GameSwapInstances[i], h.server.state.GameSwapInstances[j] = h.server.state.GameSwapInstances[j], h.server.state.GameSwapInstances[i]
		})
		// Clear players' InstanceID for a fresh assignment
		for n, p := range h.server.state.Players {
			// If player had an assigned instance, set that instance state to pending (in transition)
			if p.InstanceID != "" && p.Connected {
				// we must call setInstanceFileState without holding the main lock to avoid deadlocks
				go h.server.setInstanceFileState(p.InstanceID, types.FileStatePending)
			}
			p.InstanceID = ""
			p.Game = ""
			h.server.state.Players[n] = p
		}
		// Assign instances to players: one instance per player up to available instances
		maxAssign := min(len(players), len(h.server.state.GameSwapInstances))
		for i := 0; i < maxAssign; i++ {
			pname := players[i]
			inst := h.server.state.GameSwapInstances[i]
			p, ok := h.server.state.Players[pname]
			if !ok {
				p = types.Player{Name: pname}
			}
			p.Game = inst.Game
			p.InstanceID = inst.ID
			h.server.state.Players[pname] = p
		}
		h.server.state.UpdatedAt = time.Now()
	})
	// capture local copy of instances for sending without holding lock while network operations run
	var instances []types.GameSwapInstance
	h.server.withRLock(func() { instances = append([]types.GameSwapInstance{}, h.server.state.GameSwapInstances...) })
	playersMap := map[string]types.Player{}
	h.server.withRLock(func() {
		for n, p := range h.server.state.Players {
			playersMap[n] = p
		}
	})
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
			h.server.sendSwap(name, p.Game, p.InstanceID)
		}
	}
	return nil
}

func (h *SaveModeHandler) GetPlayer(player string) types.Player {
	// In save mode, prefer returning the first unassigned game instance.
	// We consider an instance unassigned when no player currently has its InstanceID.
	var result types.Player
	h.server.withLock(func() {
		if len(h.server.state.GameSwapInstances) > 0 {
			// build a set of assigned instance IDs
			assigned := map[string]struct{}{}
			for _, p := range h.server.state.Players {
				if p.InstanceID != "" {
					assigned[p.InstanceID] = struct{}{}
				}
			}
			// find first instance that is not assigned
			for i, inst := range h.server.state.GameSwapInstances {
				if _, ok := assigned[inst.ID]; !ok {
					// Check if save state file exists and update FileState accordingly
					savePath := filepath.Join("./saves", inst.ID+".state")
					if _, err := os.Stat(savePath); err == nil {
						// File exists, mark as ready
						h.server.state.GameSwapInstances[i].FileState = types.FileStateReady
					} else {
						// File doesn't exist, mark as none
						h.server.state.GameSwapInstances[i].FileState = types.FileStateNone
					}
					h.server.state.UpdatedAt = time.Now()
					// Persist the state change (do not hold lock while saving)
					go h.server.saveState()
					result = types.Player{
						Name:       player,
						Game:       inst.Game,
						InstanceID: inst.ID,
					}
					return
				}
			}
		}
	})
	if result.Name != "" {
		return result
	}
	return types.Player{Name: player}
}

func (h *SaveModeHandler) SetupState() error {
	return nil
}

func (h *SaveModeHandler) HandlePlayerSwap(player string, game string, instanceID string) error {
	var foundInst *types.GameSwapInstance
	var foundPlayer string
	h.server.withLock(func() {
		// If instance ID provided, assign that instance to the player (players now store InstanceID)
		if instanceID == "" {
			return
		}
		for i, inst := range h.server.state.GameSwapInstances {
			if inst.ID == instanceID {
				// capture instance
				foundInst = &h.server.state.GameSwapInstances[i]
				break
			}
		}
		for playerName, swappingPlayer := range h.server.state.Players {
			if swappingPlayer.InstanceID == instanceID && playerName != player {
				// Clear previous assignment
				swappingPlayer.Game = ""
				swappingPlayer.InstanceID = ""
				h.server.state.Players[playerName] = swappingPlayer
				if swappingPlayer.Connected {
					foundPlayer = playerName
				}
				break
			}
		}
		// update player entry if we found an instance
		if foundInst != nil {
			p, ok := h.server.state.Players[player]
			if !ok {
				p = types.Player{Name: player}
			}
			p.Game = foundInst.Game
			p.InstanceID = foundInst.ID
			h.server.state.Players[player] = p
			h.server.state.UpdatedAt = time.Now()
		}
	})
	// If instance was not provided, return an error
	if instanceID == "" {
		return errors.New("instance ID is required")
	}
	if foundInst == nil {
		return errors.New("instance not found")
	}

	_ = h.server.saveState()

	// Set instance state to pending before upload starts
	if foundPlayer != "" {
		h.server.setInstanceFileState(foundInst.ID, types.FileStatePending)
		h.server.sendSwap(foundPlayer, "", "")
	} else {
		h.server.setInstanceFileState(foundInst.ID, types.FileStateNone)
	}
	h.server.sendSwap(player, foundInst.Game, foundInst.ID)
	return nil
}

// getGameModeHandler returns the appropriate handler for the given game mode
func (s *Server) GetGameModeHandler() GameModeHandler {
	var mode types.GameMode
	s.withRLock(func() { mode = s.state.Mode })

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
