package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

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
	// Inline the previous performSwapSync implementation here so game mode
	// behavior lives with the mode handler.
	s.mu.Lock()
	players := []string{}
	for name := range s.state.Players {
		players = append(players, name)
	}
	games := append([]string{}, s.state.Games...)
	s.mu.Unlock()
	if len(players) == 0 || len(games) == 0 {
		return nil, fmt.Errorf("need players and games configured")
	}
	idx := rand.Intn(len(games))
	chosen := games[idx]
	mapping := make(map[string]string)
	for _, p := range players {
		mapping[p] = chosen
	}
	// Log the expected mapping so operators can see what we intend to do
	log.Printf("[swap][expected] mode=sync chosen=%s mapping=%+v", chosen, mapping)
	results := make(map[string]string)
	for player, game := range mapping {
		cmdID := fmt.Sprintf("sync-%d-%s", time.Now().UnixNano(), player)
		cmd := types.Command{Cmd: types.CmdSwap, ID: cmdID, Payload: map[string]string{"game": game, "mode": "sync"}}
		res, err := s.sendAndWait(player, cmd, 15*time.Second)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				results[player] = "timeout"
				log.Printf("[swap][outcome] mode=sync player=%s result=timeout cmd=%s", player, cmdID)
			} else {
				results[player] = "send_failed: " + err.Error()
				log.Printf("[swap][outcome] mode=sync player=%s result=send_failed err=%v cmd=%s", player, err, cmdID)
			}
			continue
		}
		results[player] = res
		log.Printf("[swap][outcome] mode=sync player=%s result=%s cmd=%s", player, res, cmdID)
	}
	s.mu.Lock()
	for p, g := range mapping {
		pl := s.state.Players[p]
		pl.Current = g
		s.state.Players[p] = pl
	}
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	if err := s.saveState(); err != nil {
		fmt.Printf("saveState error: %v\n", err)
	}
	return &SwapOutcome{Mapping: mapping, Results: results, DownloadResults: map[string]string{}}, nil
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
	// SaveModeHandler.HandleSwap logic outline:
	// 1. Collect list of players and available games from server state.
	// 2. Build a mapping assigning each player a game (round-robin over games).
	// 3. Send a swap command to each player, wait for ack. If any player
	//    fails to ack the swap, return the mapping and results without
	//    attempting downloads.
	// 4. If all players acknowledged, for each player determine the "owner"
	//    of the most-recent save for the assigned game. The owner is found
	//    by reading the saves/index.json and falling back to whichever
	//    connected player currently reports that game as their Current. If
	//    none found, the player is considered the owner.
	// 5. Send download requests to each player to fetch the owner's save
	//    and wait for results. Record download outcomes per player.
	// 6. Update server state with new Current games and persist state.
	//
	// This comment provides a high-level outline to help operators and
	// future contributors understand the flow.
	s.mu.Lock()
	players := []string{}
	for name := range s.state.Players {
		players = append(players, name)
	}
	games := append([]string{}, s.state.Games...)
	s.mu.Unlock()
	if len(players) == 0 || len(games) == 0 {
		return nil, fmt.Errorf("need players and games configured")
	}
	mapping := make(map[string]string)
	for i, p := range players {
		mapping[p] = games[i%len(games)]
	}
	results := make(map[string]string)
	// Log the expected mapping so operators can see what we intend to do
	log.Printf("[swap][expected] mode=save mapping=%+v", mapping)
	for player, game := range mapping {
		cmdID := fmt.Sprintf("swap-%d-%s", time.Now().UnixNano(), player)
		cmd := types.Command{Cmd: types.CmdSwap, ID: cmdID, Payload: map[string]string{"game": game}}
		res, err := s.sendAndWait(player, cmd, 20*time.Second)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				results[player] = "timeout"
				log.Printf("[swap][outcome] mode=save player=%s result=timeout cmd=%s", player, cmdID)
			} else {
				results[player] = "send_failed: " + err.Error()
				log.Printf("[swap][outcome] mode=save player=%s result=send_failed err=%v cmd=%s", player, err, cmdID)
			}
			continue
		}
		results[player] = res
		log.Printf("[swap][outcome] mode=save player=%s result=%s cmd=%s", player, res, cmdID)
	}
	failed := false
	for _, r := range results {
		if r != "ack" {
			failed = true
			break
		}
	}
	if failed {
		return &SwapOutcome{Mapping: mapping, Results: results, DownloadResults: map[string]string{}}, nil
	}
	downloadResults := make(map[string]string)
	for player, game := range mapping {
		owner := ""
		filename := game + ".state"
		indexPath := filepath.Join("./saves", "index.json")
		if b, err := os.ReadFile(indexPath); err == nil {
			var idx []SaveIndexEntry
			if json.Unmarshal(b, &idx) == nil {
				var bestAt int64
				for _, e := range idx {
					if e.Game == game || e.File == filename {
						if e.At > bestAt {
							bestAt = e.At
							owner = e.Player
						}
					}
				}
			}
		}
		if owner == "" {
			s.mu.Lock()
			for pname, p := range s.state.Players {
				if p.Current == game {
					owner = pname
					break
				}
			}
			s.mu.Unlock()
		}
		if owner == "" {
			owner = player
		}
		cmdID := fmt.Sprintf("dl-%d-%s", time.Now().UnixNano(), player)
		cmd := types.Command{Cmd: types.CmdDownloadSave, ID: cmdID, Payload: map[string]string{"player": owner, "file": filename}}
		if err := s.sendToPlayer(player, cmd); err != nil {
			downloadResults[player] = "send_failed: " + err.Error()
			log.Printf("[swap][outcome] mode=save player=%s download_send_failed err=%v cmd=%s owner=%s file=%s", player, err, cmdID, owner, filename)
			continue
		}
		res, err := s.waitForResult(cmdID, 30*time.Second)
		if err != nil {
			downloadResults[player] = "timeout"
			log.Printf("[swap][outcome] mode=save player=%s download_timeout cmd=%s owner=%s file=%s", player, cmdID, owner, filename)
		} else {
			downloadResults[player] = res
			log.Printf("[swap][outcome] mode=save player=%s download_result=%s cmd=%s owner=%s file=%s", player, res, cmdID, owner, filename)
		}
	}
	s.mu.Lock()
	for p, g := range mapping {
		pl := s.state.Players[p]
		pl.Current = g
		s.state.Players[p] = pl
	}
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	if err := s.saveState(); err != nil {
		fmt.Printf("saveState error: %v\n", err)
	}
	return &SwapOutcome{Mapping: mapping, Results: results, DownloadResults: downloadResults}, nil
}

func (h *SaveModeHandler) GetCurrentGameForPlayer(s *Server, player string) string {
	// In save mode, just return the first available game if any exist
	// Save orchestration will handle different save files per player
	if len(s.state.Games) > 0 {
		return s.state.Games[0]
	}
	return ""
}

func (h *SaveModeHandler) Description() string {
	return "Save Mode: Players play different games and perform save upload/download orchestration on swap. Each player can be assigned different games during gameplay."
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
