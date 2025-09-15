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
	var preventSame bool
	h.server.withRLock(func() { preventSame = h.server.state.PreventSameGameSwap })

	game := h.randomGame()
	if game == "" {
		return errors.New("no games available for swap")
	}

	// If preventing same game swap, check if picked game is same as current
	if preventSame {
		currentGame := ""
		h.server.withRLock(func() {
			for _, player := range h.server.state.Players {
				if player.Game != "" {
					currentGame = player.Game
					break
				}
			}
		})
		if game == currentGame {
			// Try to pick a different game
			game = h.randomGameExcluding(currentGame)
			if game == "" {
				// If no other game available, allow same game
				game = currentGame
			}
		}
	}

	// In sync mode, set all players to the same game
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		for name, player := range st.Players {
			player.Game = game
			player.InstanceID = ""
			st.Players[name] = player
		}
	})
	h.server.sendSwapAll()
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

func (h *SyncModeHandler) randomGameExcluding(exclude string) string {
	var games []string
	h.server.withRLock(func() { games = h.server.state.Games })
	if len(games) == 0 {
		return ""
	}
	// Filter out the excluded game
	var available []string
	for _, g := range games {
		if g != exclude {
			available = append(available, g)
		}
	}
	if len(available) == 0 {
		return ""
	}
	return available[rand.Intn(len(available))]
}

func (h *SyncModeHandler) GetPlayer(player string) types.Player {
	var result types.Player
	h.server.withRLock(func() {
		// If any player already has a game assigned, return that game for the requesting player.
		for _, pp := range h.server.state.Players {
			if pp.Game != "" {
				result = types.Player{Name: player, Game: pp.Game}
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

func (h *SyncModeHandler) HandlePlayerSwap(player string, game string, _ string) error {
	// In sync mode we don't use instances; just set the player's current game
	var p types.Player
	var ok bool
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		p, ok = st.Players[player]
		if !ok {
			p = types.Player{Name: player}
		}
		p.Game = game
		p.InstanceID = ""
		st.Players[player] = p
	})

	h.server.sendSwap(p)
	return nil
}

// SaveModeHandler implements the save game mode
type SaveModeHandler struct {
	server *Server
}

func (h *SaveModeHandler) HandleSwap() error {
	var waiting bool
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for range 3 {
		h.server.withRLock(func() {
			waiting = h.server.pendingInstancecount > 0
		})
		if waiting {
			h.server.RequestPendingSaves()
		}
		<-ticker.C
	}
	if waiting {
		return nil
	}

	var preventSame bool
	h.server.withRLock(func() { preventSame = h.server.state.PreventSameGameSwap })

	// Ensure there are instances to swap between
	if len(h.server.state.GameSwapInstances) == 0 {
		return errors.New("no game instances available for swap")
	}

	h.server.SetPendingAllFiles()
	// Collect player names and perform state updates under lock where necessary
	var players []string
	playerCurrentGames := make(map[string]string)
	playerCurrentInstances := make(map[string]string)
	var gameInstances []types.GameSwapInstance
	h.server.withRLock(func() {
		for name := range h.server.state.Players {
			players = append(players, name)
		}
		for n, p := range h.server.state.Players {
			playerCurrentGames[n] = p.Game
			playerCurrentInstances[n] = p.InstanceID
		}
		gameInstances = make([]types.GameSwapInstance, len(h.server.state.GameSwapInstances))
		copy(gameInstances, h.server.state.GameSwapInstances)
	})

	// Shuffle instances outside the lock
	rand.Shuffle(len(gameInstances), func(i, j int) {
		gameInstances[i], gameInstances[j] = gameInstances[j], gameInstances[i]
	})

	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		// Clear players' InstanceID for a fresh assignment
		for n, p := range st.Players {
			p.InstanceID = ""
			p.Game = ""
			st.Players[n] = p
		}
		// Assign instances to players: one instance per player up to available instances
		maxAssign := min(len(gameInstances), len(players))
		assignedInstances := make(map[int]bool) // track assigned instance indices
		for i := range maxAssign {
			pname := players[i]
			currentGame := playerCurrentGames[pname]
			currentInstance := playerCurrentInstances[pname]

			// Find an available instance, preferring one with different game if preventSame is true
			assignedIdx := -1
			if preventSame && currentGame != "" {
				// First pass: try to find instance with different game
				for j := range gameInstances {
					if !assignedInstances[j] && gameInstances[j].Game != currentGame {
						assignedIdx = j
						break
					}
				}
				// Second pass: try to find different instance if player had an instance assigned
				if assignedIdx == -1 && currentInstance != "" {
					for j := range gameInstances {
						if !assignedInstances[j] && gameInstances[j].ID != currentInstance {
							assignedIdx = j
							break
						}
					}
				}

				// If no different game found, assign any available
				if assignedIdx == -1 {
					for j := range gameInstances {
						if !assignedInstances[j] {
							assignedIdx = j
							break
						}
					}
				}
			} else {
				// Find first available instance
				for j := range gameInstances {
					if !assignedInstances[j] {
						assignedIdx = j
						break
					}
				}
			}

			if assignedIdx != -1 {
				p := st.Players[pname]
				inst := gameInstances[assignedIdx]
				p.Game = inst.Game
				p.InstanceID = inst.ID
				st.Players[pname] = p
				assignedInstances[assignedIdx] = true
			}
		}
	})
	h.server.sendSwapAll()
	return nil
}

func (h *SaveModeHandler) GetPlayer(player string) types.Player {
	// In save mode, prefer returning the first unassigned game instance.
	// We consider an instance unassigned when no player currently has its InstanceID.
	var result types.Player
	h.server.UpdateStateAndPersist(func(state *types.ServerState) {
		if len(state.GameSwapInstances) > 0 {
			// build a set of assigned instance IDs
			assigned := map[string]struct{}{}
			for _, p := range state.Players {
				if p.InstanceID != "" {
					assigned[p.InstanceID] = struct{}{}
				}
			}
			// find first instance that is not assigned
			for i, inst := range state.GameSwapInstances {
				if _, ok := assigned[inst.ID]; !ok {
					// Check if save state file exists and update FileState accordingly
					savePath := filepath.Join("./saves", inst.ID+".state")
					if _, err := os.Stat(savePath); err == nil {
						// File exists, mark as ready
						state.GameSwapInstances[i].FileState = types.FileStateReady
					} else {
						// File doesn't exist, mark as none
						state.GameSwapInstances[i].FileState = types.FileStateNone
					}
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
	var foundPlayer *types.Player
	var ok bool
	var p types.Player
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		// If instance ID provided, assign that instance to the player (players now store InstanceID)
		if instanceID == "" {
			return
		}
		for i, inst := range st.GameSwapInstances {
			if inst.ID == instanceID {
				// capture instance
				foundInst = &st.GameSwapInstances[i]
				break
			}
		}
		for playerName, swappingPlayer := range st.Players {
			if swappingPlayer.InstanceID == instanceID && playerName != player {
				// Clear previous assignment
				swappingPlayer.Game = ""
				swappingPlayer.InstanceID = ""
				st.Players[playerName] = swappingPlayer
				if swappingPlayer.Connected {
					foundPlayer = &swappingPlayer
				}
				break
			}
		}
		// update player entry if we found an instance
		if foundInst != nil {
			p, ok = st.Players[player]
			if !ok {
				p = types.Player{Name: player}
			}
			p.Game = foundInst.Game
			p.InstanceID = foundInst.ID
			st.Players[player] = p
		}
	})
	// If instance was not provided, return an error
	if instanceID == "" {
		return errors.New("instance ID is required")
	}
	if foundInst == nil {
		return errors.New("instance not found")
	}

	// Set instance state to pending before upload starts
	if foundPlayer != nil {
		h.server.setInstanceFileStateWithPlayer(foundInst.ID, types.FileStatePending, foundPlayer.Name)
		h.server.sendSwap(*foundPlayer)
	} else {
		h.server.setInstanceFileState(foundInst.ID, types.FileStateNone)
	}
	h.server.sendSwap(p)
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
