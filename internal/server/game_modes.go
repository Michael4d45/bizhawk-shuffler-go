// Package server contains game mode handlers that implement different swap behaviors.
//
// Game modes determine how players swap between games:
//   - Sync mode: All players play the same game simultaneously, swapping together.
//     Each player maintains their own save state for the shared game.
//   - Save mode: Players have individual game instances and swap save states between each other.
//     Multiple players can play the same game but with different save files.
//
// The "better random" setting (PreventSameGameSwap) controls whether players avoid
// being assigned the same game they just played, improving variety in random selections.
package server

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// InstanceSelectionCriteria holds criteria for selecting an instance
type InstanceSelectionCriteria struct {
	ExcludeInstanceIDs  map[string]bool
	ExcludeGameNames    map[string]bool
	PreferDifferentGame bool
	CurrentGame         string
	CurrentInstanceID   string
}

// InstanceCategory groups instances by availability and preference
type InstanceCategory struct {
	UnassignedDifferentGame     []string
	UnassignedDifferentInstance []string
	UnassignedSame              []string
	AssignedDifferentGame       []string
	AssignedDifferentInstance   []string
	AssignedSame                []string
}

// validateNoDuplicateInstanceAssignments checks that no two players have the same instance ID
func validateNoDuplicateInstanceAssignments(state *types.ServerState) error {
	instanceToPlayer := make(map[string]string)
	for name, player := range state.Players {
		if player.InstanceID != "" {
			if existingPlayer, exists := instanceToPlayer[player.InstanceID]; exists {
				return fmt.Errorf("duplicate instance assignment: instance %s assigned to both %s and %s",
					player.InstanceID, existingPlayer, name)
			}
			instanceToPlayer[player.InstanceID] = name
		}
	}
	return nil
}

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

// SyncModeHandler implements the sync game mode where all players play the same game
type SyncModeHandler struct {
	server *Server
}

// getCurrentGame returns the game currently being played by any player in sync mode
func (h *SyncModeHandler) getCurrentGame() string {
	var currentGame string
	h.server.withRLock(func() {
		for _, player := range h.server.state.Players {
			if player.Game != "" {
				currentGame = player.Game
				break
			}
		}
	})
	return currentGame
}

// selectGameForPlayer selects an appropriate game for a player, considering their completed games
func (h *SyncModeHandler) selectGameForPlayer(player types.Player, games []string, excludeList []string, seed int64) string {
	playerExclusions := append([]string{}, excludeList...)
	playerExclusions = append(playerExclusions, player.CompletedGames...)

	game := selectNextGame(games, playerExclusions, seed)
	if game == "" {
		log.Printf("[SyncMode] Player %s has all games completed, skipping game assignment", player.Name)
	}
	return game
}

// initializeSwapSeed ensures the swap seed is set for deterministic random selections
func (h *SyncModeHandler) initializeSwapSeed() int64 {
	var seed int64
	h.server.withRLock(func() {
		seed = h.server.state.SwapSeed
	})

	if seed == 0 {
		seed = time.Now().Unix()
		h.server.UpdateStateAndPersist(func(st *types.ServerState) {
			st.SwapSeed = seed
		})
		log.Printf("[SyncMode] Initialized swap seed to %d", seed)
	}
	return seed
}

// isGameCompletedForPlayer checks if a game is in the player's completed games list
func (h *SyncModeHandler) isGameCompletedForPlayer(player types.Player, game string) bool {
	for _, completedGame := range player.CompletedGames {
		if completedGame == game {
			return true
		}
	}
	return false
}

// HandleSwap performs a synchronized swap where all players switch to the same new game.
// In sync mode, all players play the same game simultaneously, swapping together as a group.
func (h *SyncModeHandler) HandleSwap() error {
	var preventSame bool
	var games []string
	h.server.withRLock(func() {
		preventSame = h.server.state.PreventSameGameSwap
		games = h.server.state.Games
	})

	currentGame := h.getCurrentGame()
	seed := h.initializeSwapSeed()

	// Select next game using deterministic seed
	exclude := []string{}
	if preventSame && currentGame != "" {
		exclude = append(exclude, currentGame)
	}
	game := selectNextGame(games, exclude, seed)
	if game == "" {
		// Try without exclusion if no game found with current restrictions
		game = selectNextGame(games, []string{}, seed)
		if game == "" {
			return errors.New("no games available for swap")
		}
	}

	log.Printf("[SyncMode] Selected game %s for all players (preventSame=%v, seed=%d)",
		game, preventSame, seed)

	// Increment seed for next swap
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		st.SwapSeed = seed + 1
	})

	// Assign the game to all players, handling individual completions
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		for name, player := range st.Players {
			playerGame := game
			// Check if selected game is completed for this player
			if h.isGameCompletedForPlayer(player, game) {
				// Try to find a different game excluding completed ones
				excludeList := append([]string{}, player.CompletedGames...)
				if preventSame && currentGame != "" && currentGame != game {
					excludeList = append(excludeList, currentGame)
				}
				playerGame = h.selectGameForPlayer(player, games, excludeList, seed)
				if playerGame == "" {
					// No available games for this player, skip them
					continue
				}
			}
			player.Game = playerGame
			player.InstanceID = ""
			st.Players[name] = player
			log.Printf("[SyncMode] Assigned game %s to player %s", playerGame, name)
		}
	})

	h.server.sendSwapAll()
	return nil
}

// randomGame selects a random game for new players joining sync mode
func (h *SyncModeHandler) randomGame() string {
	var games []string
	h.server.withRLock(func() {
		games = h.server.state.Games
	})
	if len(games) == 0 {
		return ""
	}

	seed := h.initializeSwapSeed()
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
func (h *SyncModeHandler) HandleRandomSwapForPlayer(playerName string) error {
	var player types.Player
	var found bool
	var preventSame bool
	var games []string

	h.server.withRLock(func() {
		preventSame = h.server.state.PreventSameGameSwap
		games = h.server.state.Games
		player, found = h.server.state.Players[playerName]
	})

	if !found {
		return fmt.Errorf("player %s not found", playerName)
	}

	seed := h.initializeSwapSeed()

	// Build exclude list
	exclude := append([]string{}, player.CompletedGames...)
	if preventSame && player.Game != "" {
		exclude = append(exclude, player.Game)
	}

	game := selectNextGame(games, exclude, seed)
	if game == "" {
		log.Printf("[SyncMode] Player %s has no available games for random swap (all completed or same game prevented)", playerName)
		return nil
	}

	log.Printf("[SyncMode] Random swap for player %s: %s -> %s (preventSame=%v)",
		playerName, player.Game, game, preventSame)

	// Increment seed for next swap
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		st.SwapSeed = seed + 1
	})

	return h.HandlePlayerSwap(playerName, game, "")
}

// SaveModeHandler implements the save game mode where players swap save states between game instances
type SaveModeHandler struct {
	server *Server
}

// buildCompletedMaps creates fast lookup maps for a player's completed games and instances
func (h *SaveModeHandler) buildCompletedMaps(player types.Player) (map[string]bool, map[string]bool) {
	completedInstances := make(map[string]bool)
	for _, ci := range player.CompletedInstances {
		completedInstances[ci] = true
	}

	completedGames := make(map[string]bool)
	for _, cg := range player.CompletedGames {
		completedGames[cg] = true
	}

	return completedInstances, completedGames
}

// clearInstanceFromPlayer removes an instance assignment from a specific player
func (h *SaveModeHandler) clearInstanceFromPlayer(instanceID string, excludePlayerName string) {
	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		for playerName, player := range st.Players {
			if player.InstanceID == instanceID && playerName != excludePlayerName {
				player.Game = ""
				player.InstanceID = ""
				st.Players[playerName] = player
				log.Printf("[SaveMode] Cleared instance %s from player %s", instanceID, playerName)
				break
			}
		}
	})
}

// findAvailableInstanceForPlayer finds the best available instance for a player based on criteria
func (h *SaveModeHandler) findAvailableInstanceForPlayer(
	player types.Player,
	gameInstances []types.GameSwapInstance,
	assignedInstances map[int]bool,
	preventSame bool,
) (int, bool) {
	completedInstances, completedGames := h.buildCompletedMaps(player)

	// Helper function to check if an instance is available
	isAvailable := func(idx int) bool {
		inst := gameInstances[idx]
		return !assignedInstances[idx] &&
			!completedInstances[inst.ID] &&
			!completedGames[inst.Game]
	}

	// Try different preference levels
	if preventSame && player.Game != "" {
		// First pass: try to find instance with different game
		for j := range gameInstances {
			inst := gameInstances[j]
			if isAvailable(j) && inst.Game != player.Game {
				return j, true
			}
		}
		// Second pass: try to find different instance (even if same game)
		if player.InstanceID != "" {
			for j := range gameInstances {
				inst := gameInstances[j]
				if isAvailable(j) && inst.ID != player.InstanceID {
					return j, true
				}
			}
		}
		// Third pass: any available instance (including same game/instance)
		for j := range gameInstances {
			if isAvailable(j) {
				return j, true
			}
		}
	} else {
		// Find first available instance
		for j := range gameInstances {
			if isAvailable(j) {
				return j, true
			}
		}
	}

	return -1, false
}

// canPlayerSwapToInstance validates whether a player can swap to a specific instance
func (h *SaveModeHandler) canPlayerSwapToInstance(player types.Player, instance types.GameSwapInstance, hasOtherPlayer bool, otherPlayer types.Player) bool {
	// Check if instance's game is completed for this player
	for _, cg := range player.CompletedGames {
		if cg == instance.Game {
			return false
		}
	}

	// Check if instance is completed for this player
	for _, ci := range player.CompletedInstances {
		if ci == instance.ID {
			return false
		}
	}

	// If swapping with another player, check if current player's game/instance is completed for the other player
	if hasOtherPlayer {
		var otherPlayerState types.Player
		h.server.withRLock(func() {
			if p, ok := h.server.state.Players[otherPlayer.Name]; ok {
				otherPlayerState = p
			}
		})

		// Check if current player's game is completed for other player
		for _, cg := range otherPlayerState.CompletedGames {
			if cg == player.Game {
				return false
			}
		}

		// Check if current player's instance is completed for other player
		if player.InstanceID != "" {
			for _, ci := range otherPlayerState.CompletedInstances {
				if ci == player.InstanceID {
					return false
				}
			}
		}
	}

	return true
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

// HandleSwap performs a full swap of all players to different game instances in save mode.
// In save mode, players are assigned to different game instances and swap save states between them.
// The "better random" setting (PreventSameGameSwap) attempts to avoid assigning the same game
// to players who just played it, improving variety.
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

	log.Printf("[SaveMode] Starting full swap (preventSame=%v)", preventSame)

	h.server.SetPendingAllFiles()

	// Collect player names and current assignments
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

	// Shuffle instances for randomness
	rand.Shuffle(len(gameInstances), func(i, j int) {
		gameInstances[i], gameInstances[j] = gameInstances[j], gameInstances[i]
	})

	h.server.UpdateStateAndPersist(func(st *types.ServerState) {
		// Clear all players' assignments for a fresh round-robin assignment
		for n, p := range st.Players {
			p.InstanceID = ""
			p.Game = ""
			st.Players[n] = p
		}

		// Assign instances to players using round-robin with preference logic
		maxAssign := min(len(gameInstances), len(players))
		assignedInstances := make(map[int]bool) // track assigned instance indices

		for i := range maxAssign {
			pname := players[i]
			player := st.Players[pname]

			// Create a temporary player object with current game/instance for preference logic
			tempPlayer := types.Player{
				Name:               player.Name,
				Game:               playerCurrentGames[pname],
				InstanceID:         playerCurrentInstances[pname],
				CompletedGames:     player.CompletedGames,
				CompletedInstances: player.CompletedInstances,
			}

			// Find the best available instance for this player
			assignedIdx, found := h.findAvailableInstanceForPlayer(tempPlayer, gameInstances, assignedInstances, preventSame)
			if found {
				inst := gameInstances[assignedIdx]
				player.Game = inst.Game
				player.InstanceID = inst.ID
				st.Players[pname] = player
				assignedInstances[assignedIdx] = true
				log.Printf("[SaveMode] Assigned instance %s (game %s) to player %s", inst.ID, inst.Game, pname)
			} else {
				log.Printf("[SaveMode] Player %s has no available instances for swap (all completed)", pname)
			}
		}

		// Validate the final state
		if err := validateNoDuplicateInstanceAssignments(st); err != nil {
			log.Printf("[SaveMode] WARNING: State validation failed after swap: %v", err)
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

// categorizeInstances groups available instances by preference level for a player
func (h *SaveModeHandler) categorizeInstances(player types.Player, preventSame bool) InstanceCategory {
	completedInstances, completedGames := h.buildCompletedMaps(player)

	playersByInstance := make(map[string]types.Player)
	h.server.withRLock(func() {
		for _, pl := range h.server.state.Players {
			if pl.InstanceID != "" {
				playersByInstance[pl.InstanceID] = pl
			}
		}
	})

	category := InstanceCategory{}

	for _, inst := range h.server.state.GameSwapInstances {
		// Skip completed instances/games
		if completedInstances[inst.ID] || completedGames[inst.Game] {
			continue
		}

		playerByInstance, hasPlayer := playersByInstance[inst.ID]

		if hasPlayer {
			// Instance is assigned to someone
			if inst.Game != player.Game {
				category.AssignedDifferentGame = append(category.AssignedDifferentGame, inst.ID)
			} else if inst.ID != player.InstanceID {
				category.AssignedDifferentInstance = append(category.AssignedDifferentInstance, inst.ID)
			} else if playerByInstance.Name != player.Name {
				category.AssignedSame = append(category.AssignedSame, inst.ID)
			}
		} else {
			// Instance is unassigned
			if inst.Game != player.Game {
				category.UnassignedDifferentGame = append(category.UnassignedDifferentGame, inst.ID)
			} else if inst.ID != player.InstanceID {
				category.UnassignedDifferentInstance = append(category.UnassignedDifferentInstance, inst.ID)
			} else {
				category.UnassignedSame = append(category.UnassignedSame, inst.ID)
			}
		}
	}

	return category
}

// getRandomInstanceForPlayer selects a random game instance for the player using priority-based selection.
// Returns the selected instance, whether it was found, the current player assigned to it (if any), and whether there's a player assigned.
// When PreventSameGameSwap is enabled, prioritizes instances with different games over same games.
// Prefers unassigned instances over assigned ones to minimize swap chains.
func (h *SaveModeHandler) getRandomInstanceForPlayer(player types.Player) (types.GameSwapInstance, bool, types.Player, bool) {
	var preventSame bool
	h.server.withRLock(func() {
		preventSame = h.server.state.PreventSameGameSwap
	})

	category := h.categorizeInstances(player, preventSame)

	// Select instance ID by priority (best to worst)
	var selectedID string
	if !preventSame {
		// When preventSame is off, allow any instance including current ones
		var allIDs []string
		allIDs = append(allIDs, category.UnassignedDifferentGame...)
		allIDs = append(allIDs, category.UnassignedDifferentInstance...)
		allIDs = append(allIDs, category.UnassignedSame...)
		allIDs = append(allIDs, category.AssignedDifferentGame...)
		allIDs = append(allIDs, category.AssignedDifferentInstance...)
		allIDs = append(allIDs, category.AssignedSame...)
		if len(allIDs) > 0 {
			selectedID = allIDs[rand.Intn(len(allIDs))]
		}
	} else {
		// Priority order: unassigned different game > unassigned different instance > unassigned same > assigned different game > assigned different instance > assigned same
		if len(category.UnassignedDifferentGame) > 0 {
			selectedID = category.UnassignedDifferentGame[rand.Intn(len(category.UnassignedDifferentGame))]
		} else if len(category.UnassignedDifferentInstance) > 0 {
			selectedID = category.UnassignedDifferentInstance[rand.Intn(len(category.UnassignedDifferentInstance))]
		} else if len(category.UnassignedSame) > 0 {
			selectedID = category.UnassignedSame[rand.Intn(len(category.UnassignedSame))]
		} else if len(category.AssignedDifferentGame) > 0 {
			selectedID = category.AssignedDifferentGame[rand.Intn(len(category.AssignedDifferentGame))]
		} else if len(category.AssignedDifferentInstance) > 0 {
			selectedID = category.AssignedDifferentInstance[rand.Intn(len(category.AssignedDifferentInstance))]
		} else if len(category.AssignedSame) > 0 {
			selectedID = category.AssignedSame[rand.Intn(len(category.AssignedSame))]
		}
	}

	if selectedID == "" {
		return types.GameSwapInstance{}, false, types.Player{}, false
	}

	// Find the instance and check if it has a player
	var instance types.GameSwapInstance
	var otherPlayer types.Player
	var hasOtherPlayer bool

	h.server.withRLock(func() {
		for _, inst := range h.server.state.GameSwapInstances {
			if inst.ID == selectedID {
				instance = inst
				break
			}
		}
		// Check if instance is assigned to someone
		for _, p := range h.server.state.Players {
			if p.InstanceID == selectedID {
				otherPlayer = p
				hasOtherPlayer = true
				break
			}
		}
	})

	return instance, true, otherPlayer, hasOtherPlayer
}

// HandleRandomSwapForPlayer performs a random swap for a specific player in save mode.
// In save mode, this can result in a chain of swaps if players need to exchange instances.
func (h *SaveModeHandler) HandleRandomSwapForPlayer(playerName string) error {
	if h.waitForFileCheck() {
		return nil
	}

	var swappedPlayers = make(map[string]bool)
	h.server.withRLock(func() {
		for name := range h.server.state.Players {
			swappedPlayers[name] = false
		}
	})

	for {
		var player types.Player
		var found bool
		h.server.withRLock(func() {
			player, found = h.server.state.Players[playerName]
		})
		if !found {
			return fmt.Errorf("player %s not found", playerName)
		}

		instance, hasInstance, otherPlayer, hasOtherPlayer := h.getRandomInstanceForPlayer(player)
		if !hasInstance {
			log.Printf("[SaveMode] Player %s has no available instances for random swap (all completed)", playerName)
			break
		}

		// Validate that this swap is allowed
		if !h.canPlayerSwapToInstance(player, instance, hasOtherPlayer, otherPlayer) {
			log.Printf("[SaveMode] Player %s cannot swap to instance %s (game %s) - validation failed",
				playerName, instance.ID, instance.Game)
			break
		}

		log.Printf("[SaveMode] Swapping player %s to instance %s (game %s)",
			playerName, instance.ID, instance.Game)

		h.server.setPlayerFilePending(player)

		player.InstanceID = instance.ID
		player.Game = instance.Game

		h.server.UpdateStateAndPersist(func(st *types.ServerState) {
			// Clear instance from previous owner if swapping with another player
			if hasOtherPlayer {
				h.clearInstanceFromPlayer(instance.ID, playerName)
			}
			// Assign to current player
			st.Players[player.Name] = player
			// Validate no duplicates after assignment
			if err := validateNoDuplicateInstanceAssignments(st); err != nil {
				log.Printf("[SaveMode] WARNING: State validation failed: %v", err)
			}
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
