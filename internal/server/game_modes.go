package server

import (
	"errors"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// selectNextGame selects the next game from available games using deterministic random with seed.
// This function is abstracted to support future ordering modes (e.g., sequential, custom).
// It excludes games in the exclude list.
func selectNextGame(availableGames []string, exclude []string, seed int64) string {
	if len(availableGames) == 0 {
		return ""
	}

	// Build exclusion map for fast lookup
	excludeMap := make(map[string]bool)
	for _, g := range exclude {
		excludeMap[g] = true
	}

	// Filter available games
	var filtered []string
	for _, g := range availableGames {
		if !excludeMap[g] {
			filtered = append(filtered, g)
		}
	}

	if len(filtered) == 0 {
		return ""
	}

	// Use deterministic random with seed
	rng := rand.New(rand.NewSource(seed))
	return filtered[rng.Intn(len(filtered))]
}

// GameModeHandler defines the interface for implementing game mode behavior
type GameModeHandler interface {
	// HandleSwap performs the swap operation for this game mode
	HandleSwap() error

	// GetPlayer determines what game a player should be playing in this mode
	GetPlayer(player string) types.Player

	SetupState() error

	// HandlePlayerSwap updates server state for a player-level swap (assign instances, set player->game mapping, etc)
	HandlePlayerSwap(player string, game string, instanceID string) error

	// Perform a random swap for a specific player
	HandleRandomSwapForPlayer(param1 string) error
}

// SyncModeHandler implements the sync game mode
type SyncModeHandler struct {
	server *Server
}

func (h *SyncModeHandler) HandleSwap() error {
	var preventSame bool
	var games []string
	var currentGame string
	var seed int64
	h.server.withRLock(func() {
		preventSame = h.server.state.PreventSameGameSwap
		games = h.server.state.Games
		seed = h.server.state.SwapSeed
		for _, player := range h.server.state.Players {
			if player.Game != "" {
				currentGame = player.Game
				break
			}
		}
	})

	// Initialize seed if not set
	if seed == 0 {
		seed = time.Now().Unix()
		h.server.UpdateStateAndPersist(func(st *types.ServerState) {
			st.SwapSeed = seed
		})
	}

	// Select next game using deterministic seed
	exclude := []string{}
	if preventSame && currentGame != "" {
		exclude = append(exclude, currentGame)
	}
	game := selectNextGame(games, exclude, seed)
	if game == "" {
		// Try without exclusion
		game = selectNextGame(games, []string{}, seed)
		if game == "" {
			return errors.New("no games available for swap")
		}
	}

	// Increment seed for next swap
	newSeed := seed + 1
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		st.SwapSeed = newSeed
	})

	// In sync mode, check completed games per player and assign accordingly
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		for name, player := range st.Players {
			playerGame := game
			// Check if selected game is completed for this player
			completed := false
			for _, cg := range player.CompletedGames {
				if cg == game {
					completed = true
					break
				}
			}
			if completed {
				// Try to find a different game excluding completed ones
				excludeList := append([]string{}, player.CompletedGames...)
				if preventSame && currentGame != "" && currentGame != game {
					excludeList = append(excludeList, currentGame)
				}
				playerGame = selectNextGame(games, excludeList, seed)
				if playerGame == "" {
					// No available games for this player, skip them
					log.Printf("Player %s has all games completed, skipping swap", name)
					continue
				}
			}
			player.Game = playerGame
			player.InstanceID = ""
			st.Players[name] = player
		}
	})
	h.server.sendSwapAll()
	return nil
}

func (h *SyncModeHandler) randomGame() string {
	var games []string
	var seed int64
	h.server.withRLock(func() {
		games = h.server.state.Games
		seed = h.server.state.SwapSeed
	})
	if len(games) == 0 {
		return ""
	}
	// Initialize seed if not set
	if seed == 0 {
		seed = time.Now().Unix()
		h.server.UpdateStateAndPersist(func(st *types.ServerState) {
			st.SwapSeed = seed
		})
	}
	return selectNextGame(games, []string{}, seed)
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
	// add all gaemes in catalog to available games if not already present
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		existing := make(map[string]bool)
		for _, g := range st.Games {
			existing[g] = true
		}
		for _, entry := range st.MainGames {
			if !existing[entry.File] {
				st.Games = append(st.Games, entry.File)
				existing[entry.File] = true
			}
		}
	})

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

// HandleRandomSwapForPlayer performs a random swap for a specific player in sync mode
func (h *SyncModeHandler) HandleRandomSwapForPlayer(player string) error {
	var preventSame bool
	var games []string
	var currentGame string
	var completedGames []string
	var seed int64
	h.server.withRLock(func() {
		preventSame = h.server.state.PreventSameGameSwap
		games = h.server.state.Games
		seed = h.server.state.SwapSeed
		if p, ok := h.server.state.Players[player]; ok {
			currentGame = p.Game
			completedGames = append([]string{}, p.CompletedGames...)
		}
	})

	// Initialize seed if not set
	if seed == 0 {
		seed = time.Now().Unix()
		h.server.UpdateStateAndPersist(func(st *types.ServerState) {
			st.SwapSeed = seed
		})
	}

	// Build exclude list
	exclude := append([]string{}, completedGames...)
	if preventSame && currentGame != "" {
		exclude = append(exclude, currentGame)
	}

	game := selectNextGame(games, exclude, seed)
	if game == "" {
		log.Printf("Player %s has no available games for swap (all completed or same game prevented)", player)
		return nil
	}

	// Increment seed for next swap
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		st.SwapSeed = seed + 1
	})

	return h.HandlePlayerSwap(player, game, "")
}

// SaveModeHandler implements the save game mode
type SaveModeHandler struct {
	server *Server
}

func (h *SaveModeHandler) waitForFileCheck() bool {
	var waitingForPendingFiles bool
	var waitingForPendingCommands bool
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for range 3 {
		h.server.withRLock(func() {
			waitingForPendingFiles = h.server.pendingInstancecount > 0
			waitingForPendingCommands = len(h.server.pending) > 0
		})
		if waitingForPendingFiles {
			h.server.RequestPendingSaves()
		} else if !waitingForPendingCommands {
			break
		}
		<-ticker.C
	}
	if waitingForPendingFiles {
		log.Printf("waiting for %d pending file checks to complete\n", h.server.pendingInstancecount)
	}
	if waitingForPendingCommands {
		h.server.withRLock(func() {
			for name, p := range h.server.pending {
				log.Printf("waiting for pending command check: %s: %v\n", name, p)
			}
		})
	}
	return waitingForPendingFiles || waitingForPendingCommands
}

func (h *SaveModeHandler) HandleSwap() error {
	if h.waitForFileCheck() {
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
			p := st.Players[pname]
			currentGame := playerCurrentGames[pname]
			currentInstance := playerCurrentInstances[pname]

			// Build completed maps for this player
			completedInstances := make(map[string]bool)
			for _, ci := range p.CompletedInstances {
				completedInstances[ci] = true
			}
			completedGames := make(map[string]bool)
			for _, cg := range p.CompletedGames {
				completedGames[cg] = true
			}

			// Find an available instance, preferring one with different game if preventSame is true
			// and excluding completed instances/games
			assignedIdx := -1
			if preventSame && currentGame != "" {
				// First pass: try to find instance with different game and not completed
				for j := range gameInstances {
					inst := gameInstances[j]
					if !assignedInstances[j] && inst.Game != currentGame &&
						!completedInstances[inst.ID] && !completedGames[inst.Game] {
						assignedIdx = j
						break
					}
				}
				// Second pass: try to find different instance if player had an instance assigned
				if assignedIdx == -1 && currentInstance != "" {
					for j := range gameInstances {
						inst := gameInstances[j]
						if !assignedInstances[j] && inst.ID != currentInstance &&
							!completedInstances[inst.ID] && !completedGames[inst.Game] {
							assignedIdx = j
							break
						}
					}
				}

				// If no different game found, assign any available non-completed
				if assignedIdx == -1 {
					for j := range gameInstances {
						inst := gameInstances[j]
						if !assignedInstances[j] &&
							!completedInstances[inst.ID] && !completedGames[inst.Game] {
							assignedIdx = j
							break
						}
					}
				}
			} else {
				// Find first available instance that's not completed
				for j := range gameInstances {
					inst := gameInstances[j]
					if !assignedInstances[j] &&
						!completedInstances[inst.ID] && !completedGames[inst.Game] {
						assignedIdx = j
						break
					}
				}
			}

			if assignedIdx != -1 {
				inst := gameInstances[assignedIdx]
				p.Game = inst.Game
				p.InstanceID = inst.ID
				st.Players[pname] = p
				assignedInstances[assignedIdx] = true
			} else {
				log.Printf("Player %s has no available instances for swap (all completed)", pname)
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
	// Ensure there are instances for all games in the catalog
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		existing := make(map[string]bool)
		for _, inst := range st.GameSwapInstances {
			existing[inst.Game] = true
		}

		// Build a map of existing instance IDs for counter-based naming
		existingIDs := make(map[string]bool)
		for _, inst := range st.GameSwapInstances {
			existingIDs[inst.ID] = true
		}

		for _, entry := range st.MainGames {
			if !existing[entry.File] {
				newInst := types.GameSwapInstance{
					ID:        generateInstanceID(entry.File, existingIDs),
					Game:      entry.File,
					FileState: types.FileStateNone,
				}
				// Track the new ID so subsequent instances of the same game can increment
				existingIDs[newInst.ID] = true
				st.GameSwapInstances = append(st.GameSwapInstances, newInst)
				existing[entry.File] = true
			}
		}
	})
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

// getRandomInstanceForPlayer selects a random game instance for the player, avoiding their current game if possible
func (h *SaveModeHandler) getRandomInstanceForPlayer(player types.Player) (types.GameSwapInstance, bool, types.Player, bool) {
	// First, try to find instances with different games than the player's current game
	// and that are not currently assigned to any player
	// Also filter out completed instances for this player
	var (
		preventSame                                    bool
		playersByInstance                              = make(map[string]types.Player)
		instanceIDs                                    []string
		instancesWithPlayer                            []string
		instancesWithPlayerWithoutPlayersGame          []string
		instancesWithPlayerWithoutPlayersInstanceID    []string
		instancesWithoutPlayer                         []string
		instancesWithoutPlayerWithoutPlayersGame       []string
		instancesWithoutPlayerWithoutPlayersInstanceID []string
		completedInstances                             = make(map[string]bool)
	)
	// Build completed instances map for fast lookup
	for _, ci := range player.CompletedInstances {
		completedInstances[ci] = true
	}
	// Also check completed games - if an instance's game is completed, exclude the instance
	completedGames := make(map[string]bool)
	for _, cg := range player.CompletedGames {
		completedGames[cg] = true
	}

	h.server.withRLock(func() {
		preventSame = h.server.state.PreventSameGameSwap
		for _, pl := range h.server.state.Players {
			if pl.InstanceID != "" {
				playersByInstance[pl.InstanceID] = pl
			}
		}
		for _, inst := range h.server.state.GameSwapInstances {
			// Skip if instance is completed for this player
			if completedInstances[inst.ID] {
				continue
			}
			// Skip if instance's game is completed for this player
			if completedGames[inst.Game] {
				continue
			}
			instanceIDs = append(instanceIDs, inst.ID)
			if playerByInstance, ok := playersByInstance[inst.ID]; ok {
				if inst.Game != player.Game {
					instancesWithPlayerWithoutPlayersGame = append(instancesWithPlayerWithoutPlayersGame, inst.ID)
				} else if inst.ID != player.InstanceID {
					instancesWithPlayerWithoutPlayersInstanceID = append(instancesWithPlayerWithoutPlayersInstanceID, inst.ID)
				} else if playerByInstance.Name != player.Name {
					instancesWithPlayer = append(instancesWithPlayer, inst.ID)
				}
			} else {
				if inst.Game != player.Game {
					instancesWithoutPlayerWithoutPlayersGame = append(instancesWithoutPlayerWithoutPlayersGame, inst.ID)
				} else if inst.ID != player.InstanceID {
					instancesWithoutPlayerWithoutPlayersInstanceID = append(instancesWithoutPlayerWithoutPlayersInstanceID, inst.ID)
				} else {
					instancesWithoutPlayer = append(instancesWithoutPlayer, inst.ID)
				}
			}
		}
	})

	var instanceID string
	if !preventSame && len(instanceIDs) > 0 {
		instanceID = instanceIDs[rand.Intn(len(instanceIDs))]
	} else if len(instancesWithoutPlayerWithoutPlayersGame) > 0 {
		instanceID = instancesWithoutPlayerWithoutPlayersGame[rand.Intn(len(instancesWithoutPlayerWithoutPlayersGame))]
	} else if len(instancesWithoutPlayerWithoutPlayersInstanceID) > 0 {
		instanceID = instancesWithoutPlayerWithoutPlayersInstanceID[rand.Intn(len(instancesWithoutPlayerWithoutPlayersInstanceID))]
	} else if len(instancesWithoutPlayer) > 0 {
		instanceID = instancesWithoutPlayer[rand.Intn(len(instancesWithoutPlayer))]
	} else if len(instancesWithPlayerWithoutPlayersGame) > 0 {
		instanceID = instancesWithPlayerWithoutPlayersGame[rand.Intn(len(instancesWithPlayerWithoutPlayersGame))]
	} else if len(instancesWithPlayerWithoutPlayersInstanceID) > 0 {
		instanceID = instancesWithPlayerWithoutPlayersInstanceID[rand.Intn(len(instancesWithPlayerWithoutPlayersInstanceID))]
	} else if len(instancesWithPlayer) > 0 {
		instanceID = instancesWithPlayer[rand.Intn(len(instancesWithPlayer))]
	} else {
		return types.GameSwapInstance{}, false, types.Player{}, false
	}
	otherPlayer, hasOtherPlayer := playersByInstance[instanceID]
	var instance types.GameSwapInstance
	h.server.withRLock(func() {
		for _, inst := range h.server.state.GameSwapInstances {
			if inst.ID == instanceID {
				instance = inst
				break
			}
		}
	})
	return instance, instance.ID != "", otherPlayer, hasOtherPlayer
}

// HandleRandomSwapForPlayer performs a random swap for a specific player in save mode
func (h *SaveModeHandler) HandleRandomSwapForPlayer(playerName string) error {
	if h.waitForFileCheck() {
		return nil
	}

	var (
		player         types.Player
		foundPlayer    bool
		swappedPlayers = make(map[string]bool)
	)
	// Read current state
	h.server.withRLock(func() {
		for name := range h.server.state.Players {
			swappedPlayers[name] = false
		}
	})

	for {
		// Refresh player data
		h.server.withRLock(func() {
			player, foundPlayer = h.server.state.Players[playerName]
		})
		if !foundPlayer {
			return errors.New("player not found: " + playerName)
		}

		instance, hasInstance, otherPlayer, hasOtherPlayer := h.getRandomInstanceForPlayer(player)
		if !hasInstance {
			log.Printf("Player %s has no available instances for swap (all completed)", playerName)
			break
		}

		// Check if instance's game is completed for Player A
		gameCompleted := false
		for _, cg := range player.CompletedGames {
			if cg == instance.Game {
				gameCompleted = true
				break
			}
		}
		instanceCompleted := false
		for _, ci := range player.CompletedInstances {
			if ci == instance.ID {
				instanceCompleted = true
				break
			}
		}
		if gameCompleted || instanceCompleted {
			log.Printf("Player %s cannot swap to instance %s (game %s) - completed", playerName, instance.ID, instance.Game)
			break
		}

		// If swapping with another player, check if Player A's current game is completed for Player B
		if hasOtherPlayer {
			var otherPlayerCompleted bool
			h.server.withRLock(func() {
				if op, ok := h.server.state.Players[otherPlayer.Name]; ok {
					// Check if Player A's current game is completed for Player B
					for _, cg := range op.CompletedGames {
						if cg == player.Game {
							otherPlayerCompleted = true
							break
						}
					}
					// Check if Player A's current instance is completed for Player B
					if player.InstanceID != "" {
						for _, ci := range op.CompletedInstances {
							if ci == player.InstanceID {
								otherPlayerCompleted = true
								break
							}
						}
					}
				}
			})
			if otherPlayerCompleted {
				log.Printf("Player %s cannot swap with %s - Player A's game/instance is completed for Player B", playerName, otherPlayer.Name)
				break
			}
		}

		h.server.setPlayerFilePending(player)

		player.InstanceID = instance.ID
		player.Game = instance.Game
		h.server.UpdateStateAndPersist(func(st *types.ServerState) {
			st.Players[player.Name] = player
		})
		h.server.sendSwap(player)
		swappedPlayers[player.Name] = true

		if !hasOtherPlayer {
			break
		}
		playerName = otherPlayer.Name
		if swappedPlayers[playerName] {
			break
		}
	}

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

// generateInstanceID creates a unique instance ID for a game file using counter-based naming
func generateInstanceID(gameFile string, existingIDs map[string]bool) string {
	// Extract a portion of the game file name (remove extension and take first part)
	nameWithoutExt := strings.TrimSuffix(gameFile, filepath.Ext(gameFile))

	// Take first 20 characters of the name, replace spaces/special chars with hyphens
	cleanName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, nameWithoutExt)

	// Limit to 20 characters
	if len(cleanName) > 20 {
		cleanName = cleanName[:20]
	}

	// Convert to lowercase for consistency
	cleanName = strings.ToLower(cleanName)

	// Check if base name exists, if not use it directly
	if existingIDs == nil || !existingIDs[cleanName] {
		return cleanName
	}

	// Increment counter until we find a free name
	counter := 1
	for {
		candidate := cleanName + "-" + strconv.Itoa(counter)
		if existingIDs == nil || !existingIDs[candidate] {
			return candidate
		}
		counter++
	}
}
