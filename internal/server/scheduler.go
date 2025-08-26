package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// SwapOutcome groups results from performSwap so callers can inspect mapping and per-player results
type SwapOutcome struct {
	Mapping         map[string]string
	Results         map[string]string
	DownloadResults map[string]string
}

// performSwap dispatches to the appropriate mode implementation.
func (s *Server) performSwap() (*SwapOutcome, error) {
	s.mu.Lock()
	mode := s.state.Mode
	s.mu.Unlock()
	
	handler, err := getGameModeHandler(mode)
	if err != nil {
		return nil, err
	}
	
	return handler.HandleSwap(s)
}

// performSwapSync implements the "sync" mode.
func (s *Server) performSwapSync() (*SwapOutcome, error) {
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
	results := make(map[string]string)
	for player, game := range mapping {
		cmdID := fmt.Sprintf("sync-%d-%s", time.Now().UnixNano(), player)
		cmd := types.Command{Cmd: types.CmdSwap, ID: cmdID, Payload: map[string]string{"game": game, "mode": "sync"}}
		res, err := s.sendAndWait(player, cmd, 15*time.Second)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				results[player] = "timeout"
			} else {
				results[player] = "send_failed: " + err.Error()
			}
			continue
		}
		results[player] = res
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

// performSwapSave implements the save-swap mode.
func (s *Server) performSwapSave() (*SwapOutcome, error) {
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
	for player, game := range mapping {
		cmdID := fmt.Sprintf("swap-%d-%s", time.Now().UnixNano(), player)
		cmd := types.Command{Cmd: types.CmdSwap, ID: cmdID, Payload: map[string]string{"game": game}}
		res, err := s.sendAndWait(player, cmd, 20*time.Second)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				results[player] = "timeout"
			} else {
				results[player] = "send_failed: " + err.Error()
			}
			continue
		}
		results[player] = res
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
			continue
		}
		res, err := s.waitForResult(cmdID, 30*time.Second)
		if err != nil {
			downloadResults[player] = "timeout"
		} else {
			downloadResults[player] = res
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

// schedulerLoop schedules automatic swaps when enabled.
func (s *Server) schedulerLoop() {
	for {
		s.mu.Lock()
		running := s.state.Running
		enabled := s.state.SwapEnabled
		s.mu.Unlock()
		if !running || !enabled {
			<-s.schedulerCh
			continue
		}
		s.mu.Lock()
		minv := s.state.MinIntervalSecs
		maxv := s.state.MaxIntervalSecs
		s.mu.Unlock()
		var interval int
		if minv > 0 && maxv > 0 && maxv >= minv {
			interval = minv + rand.Intn(maxv-minv+1)
		} else if minv > 0 {
			interval = minv
		} else if maxv > 0 {
			interval = maxv
		} else {
			interval = 300
		}
		nextAt := time.Now().Add(time.Duration(interval) * time.Second).Unix()
		s.mu.Lock()
		s.state.NextSwapAt = nextAt
		s.state.UpdatedAt = time.Now()
		s.mu.Unlock()
		if err := s.saveState(); err != nil {
			fmt.Printf("saveState error: %v\n", err)
		}
		s.broadcast(types.Command{Cmd: types.CmdStateUpdate, Payload: map[string]any{"next_swap_at": nextAt, "updated_at": time.Now()}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
		timer := time.NewTimer(time.Duration(interval) * time.Second)
		select {
		case <-timer.C:
		case <-s.schedulerCh:
			if !timer.Stop() {
				<-timer.C
			}
			continue
		}
		s.mu.Lock()
		if !s.state.Running || !s.state.SwapEnabled {
			s.mu.Unlock()
			continue
		}
		s.mu.Unlock()
		go func() {
			_, err := s.performSwap()
			if err != nil {
				fmt.Printf("performSwap error: %v\n", err)
			}
			select {
			case s.schedulerCh <- struct{}{}:
			default:
			}
		}()
	}
}
