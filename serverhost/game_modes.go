// package serverhost contains game mode handlers that implement different swap behaviors.
//
// Game modes determine how players swap between games:
//   - Sync mode: All players play the same game simultaneously, swapping together.
//     Each player maintains their own save state for the shared game.
//   - Save mode: Players have individual game instances and swap save states between each other.
//     Multiple players can play the same game but with different save files.
//
// The "better random" setting (PreventSameGameSwap) controls whether players avoid
// being assigned the same game they just played, improving variety in random selections.
package serverhost

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
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
func validateNoDuplicateInstanceAssignments(state *protocol.ServerState) error {
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
	GetPlayer(player string) protocol.Player

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
func (h *SyncModeHandler) selectGameForPlayer(player protocol.Player, games []string, excludeList []string, seed int64) string {
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
		h.server.UpdateStateAndPersist(func(st *protocol.ServerState) {
			st.SwapSeed = seed
		})
		log.Printf("[SyncMode] Initialized swap seed to %d", seed)
	}
	return seed
}

// isGameCompletedForPlayer checks if a game is in the player's completed games list
func (h *SyncModeHandler) isGameCompletedForPlayer(player protocol.Player, game string) bool {
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
	h.server.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.SwapSeed = seed + 1
	})

	// Assign the game to all players, handling individual completions
	h.server.UpdateStateAndPersist(func(st *protocol.ServerState) {
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

	h.server.sendSwapAll(SwapSendOptions{})
	return nil
}

func (h *SyncModeHandler) GetPlayer(player string) protocol.Player {
	seed := h.initializeSwapSeed()
	var result protocol.Player
	h.server.withRLock(func() {
		// If any player already has a game assigned, return that game for the requesting player.
		for _, pp := range h.server.state.Players {
			if pp.Game != "" {
				result = protocol.Player{Name: player, Game: pp.Game}
				return
			}
		}
		// Otherwise pick a random game from the available games
		if len(h.server.state.Games) > 0 {
			result = protocol.Player{Name: player, Game: selectNextGame(h.server.state.Games, []string{}, seed)}
			return
		}
		result = protocol.Player{Name: player}
	})
	return result
}

func (h *SyncModeHandler) SetupState() error {
	// add all gaemes in catalog to available games if not already present
	h.server.UpdateStateAndPersist(func(st *protocol.ServerState) {
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
	var p protocol.Player
	var ok bool
	h.server.UpdateStateAndPersist(func(st *protocol.ServerState) {
		p, ok = st.Players[player]
		if !ok {
			p = protocol.Player{Name: player}
		}
		p.Game = game
		p.InstanceID = ""
		st.Players[player] = p
	})

	h.server.sendSwap(p, SwapSendOptions{})
	return nil
}

// HandleRandomSwapForPlayer performs a random swap for a specific player in sync mode
func (h *SyncModeHandler) HandleRandomSwapForPlayer(playerName string) error {
	var player protocol.Player
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
	h.server.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.SwapSeed = seed + 1
	})

	return h.HandlePlayerSwap(playerName, game, "")
}

// SaveModeHandler implements the save game mode where players swap save states between game instances
type SaveModeHandler struct {
	server *Server
}

// buildCompletedMaps creates fast lookup maps for a player's completed games and instances
func (h *SaveModeHandler) buildCompletedMaps(player protocol.Player) (map[string]bool, map[string]bool) {
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

// findAvailableInstanceForPlayer finds the best available instance for a player based on criteria
func (h *SaveModeHandler) findAvailableInstanceForPlayer(
	player protocol.Player,
	gameInstances []protocol.GameSwapInstance,
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

// waitForFileCheck waits until no pending save files or in-flight swap commands (TS parity: 30s).
func (h *SaveModeHandler) waitForFileCheck() bool {
	return h.waitForSwapGate(30 * time.Second)
}

func (h *SaveModeHandler) waitForSwapGate(timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var waitingFiles, waitingCmds bool
		h.server.withRLock(func() {
			waitingFiles = h.server.pendingInstancecount > 0
			waitingCmds = len(h.server.pending) > 0
		})
		if !waitingFiles && !waitingCmds {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
	var still bool
	h.server.withRLock(func() {
		still = h.server.pendingInstancecount > 0 || len(h.server.pending) > 0
		if still {
			log.Printf("[SaveMode] waitForSwapGate timed out: pendingInstancecount=%d pendingCmds=%d",
				h.server.pendingInstancecount, len(h.server.pending))
		}
	})
	return still
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
	h.server.RequestPendingSaves()
	if h.server.WaitForPendingSaves(60 * time.Second) {
		log.Printf("[SaveMode] timed out waiting for players to upload saves before mass swap")
		return nil
	}

	// Collect player names and current assignments
	var players []string
	playerCurrentGames := make(map[string]string)
	playerCurrentInstances := make(map[string]string)
	var gameInstances []protocol.GameSwapInstance

	h.server.withRLock(func() {
		for name := range h.server.state.Players {
			players = append(players, name)
		}
		for n, p := range h.server.state.Players {
			playerCurrentGames[n] = p.Game
			playerCurrentInstances[n] = p.InstanceID
		}
		gameInstances = make([]protocol.GameSwapInstance, len(h.server.state.GameSwapInstances))
		copy(gameInstances, h.server.state.GameSwapInstances)
	})

	// Shuffle instances for randomness
	rand.Shuffle(len(gameInstances), func(i, j int) {
		gameInstances[i], gameInstances[j] = gameInstances[j], gameInstances[i]
	})

	h.server.UpdateStateAndPersist(func(st *protocol.ServerState) {
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
			tempPlayer := protocol.Player{
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

	h.server.sendSwapAll(SwapSendOptions{SkipSave: true})
	return nil
}

func (h *SaveModeHandler) GetPlayer(player string) protocol.Player {
	var result protocol.Player
	h.server.withRLock(func() {
		assigned := map[string]struct{}{}
		for _, p := range h.server.state.Players {
			if p.InstanceID != "" {
				assigned[p.InstanceID] = struct{}{}
			}
		}
		for _, inst := range h.server.state.GameSwapInstances {
			if _, ok := assigned[inst.ID]; ok {
				continue
			}
			result = protocol.Player{
				Name:       player,
				Game:       inst.Game,
				InstanceID: inst.ID,
			}
			return
		}
	})
	if result.Name != "" {
		return result
	}
	return protocol.Player{Name: player}
}

func (h *SaveModeHandler) SetupState() error {
	h.server.UpdateStateAndPersist(func(st *protocol.ServerState) {
		updated := protocol.SetupSaveState(*st)
		st.GameSwapInstances = updated.GameSwapInstances
	})
	return nil
}

func (h *SaveModeHandler) HandlePlayerSwap(player string, game string, instanceID string) error {
	if instanceID == "" {
		h.server.UpdateStateAndPersist(func(st *protocol.ServerState) {
			p, ok := st.Players[player]
			if !ok {
				return
			}
			p.Game = ""
			p.InstanceID = ""
			st.Players[player] = p
		})
		return nil
	}

	var foundInst *protocol.GameSwapInstance
	var foundPlayer *protocol.Player
	var ok bool
	var p protocol.Player
	h.server.UpdateStateAndPersist(func(st *protocol.ServerState) {
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
				p = protocol.Player{Name: player}
			}
			p.Game = foundInst.Game
			p.InstanceID = foundInst.ID
			st.Players[player] = p
		}
	})
	if foundInst == nil {
		return errors.New("instance not found")
	}

	if foundPlayer != nil {
		h.server.setInstanceFileStateWithPlayer(foundInst.ID, protocol.FileStatePending, foundPlayer.Name)
		h.server.RequestPendingSaves()
		if h.server.WaitForPendingSaves(60 * time.Second) {
			log.Printf("[SaveMode] timed out waiting for displaced player %s save", foundPlayer.Name)
			return nil
		}
		h.server.sendSwap(*foundPlayer, SwapSendOptions{SkipSave: true})
	} else {
		h.server.setInstanceFileState(foundInst.ID, protocol.FileStateNone)
	}
	h.server.sendSwap(p, SwapSendOptions{SkipSave: true})
	return nil
}

// categorizeInstances groups available instances by preference level for a player
func (h *SaveModeHandler) categorizeInstances(player protocol.Player, preventSame bool) InstanceCategory {
	completedInstances, completedGames := h.buildCompletedMaps(player)

	playersByInstance := make(map[string]protocol.Player)
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
func (h *SaveModeHandler) getRandomInstanceForPlayer(player protocol.Player) (protocol.GameSwapInstance, bool, protocol.Player, bool) {
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
		return protocol.GameSwapInstance{}, false, protocol.Player{}, false
	}

	// Find the instance and check if it has a player
	var instance protocol.GameSwapInstance
	var otherPlayer protocol.Player
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

// HandleRandomSwapForPlayer performs a random swap for a specific player in save mode (TS parity).
func (h *SaveModeHandler) HandleRandomSwapForPlayer(playerName string) error {
	if h.waitForFileCheck() {
		return nil
	}

	pending := make(map[string]bool)
	h.server.withRLock(func() {
		for name := range h.server.state.Players {
			pending[name] = true
		}
	})

	current := playerName
	playerCount := len(pending)
	for step := 0; step < playerCount+1; step++ {
		var player protocol.Player
		var found bool
		h.server.withRLock(func() {
			player, found = h.server.state.Players[current]
		})
		if !found {
			return fmt.Errorf("player %s not found", current)
		}

		instance, hasInstance, otherPlayer, hasOtherPlayer := h.getRandomInstanceForPlayer(player)
		if !hasInstance {
			log.Printf("[SaveMode] Player %s has no available instances for random swap", current)
			break
		}

		if hasOtherPlayer && otherPlayer.Connected && otherPlayer.InstanceID != "" {
			h.server.setInstanceFileStateWithPlayer(
				otherPlayer.InstanceID, protocol.FileStatePending, otherPlayer.Name)
		}
		h.server.setPlayerFilePending(player)
		h.server.RequestPendingSaves()
		if h.server.WaitForPendingSaves(60 * time.Second) {
			log.Printf("[SaveMode] timed out waiting for random-swap saves")
			return nil
		}

		player.InstanceID = instance.ID
		player.Game = instance.Game
		h.server.UpdateStateAndPersist(func(st *protocol.ServerState) {
			if hasOtherPlayer {
				for name, pl := range st.Players {
					if pl.InstanceID == instance.ID && name != player.Name {
						pl.Game = ""
						pl.InstanceID = ""
						st.Players[name] = pl
					}
				}
			}
			st.Players[player.Name] = player
			if err := validateNoDuplicateInstanceAssignments(st); err != nil {
				log.Printf("[SaveMode] WARNING: State validation failed: %v", err)
			}
		})

		h.server.sendSwap(player, SwapSendOptions{SkipSave: true})
		delete(pending, player.Name)

		if !hasOtherPlayer {
			break
		}
		chain := otherPlayer.Name
		if !pending[chain] {
			break
		}
		current = chain
	}

	return nil
}

// getGameModeHandler returns the appropriate handler for the given game mode
func (s *Server) GetGameModeHandler() GameModeHandler {
	var mode protocol.GameMode
	s.withRLock(func() { mode = s.state.Mode })

	switch mode {
	case protocol.GameModeSync:
		return &SyncModeHandler{
			server: s,
		}
	case protocol.GameModeSave:
		return &SaveModeHandler{
			server: s,
		}
	default:
		panic("unexpected game mode: \"" + mode + "\"")
	}
}
