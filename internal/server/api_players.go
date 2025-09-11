package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// apiSwapPlayer: POST {player:..., instance_id:...}
// If instance_id is provided, assign that instance to the player and swap to its game.
func (s *Server) apiSwapPlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Player     string `json:"player"`
		InstanceID string `json:"instance_id"`
		Game       string `json:"game"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Determine the game file to swap to. Prefer explicit game in request, otherwise use instance lookup.
	var gameFile string
	if b.Game != "" {
		gameFile = b.Game
		// ensure player exists
		s.UpdateStateAndPersist(func(st *types.ServerState) {
			if _, ok := st.Players[b.Player]; !ok {
				st.Players[b.Player] = types.Player{Name: b.Player}
			}
		})
	} else if b.InstanceID != "" {
		// Look up instance by id and assign to player if provided
		var found bool
		// Find instance using snapshot of instances (no write lock needed)
		_, _, instances := s.SnapshotGames()
		for _, inst := range instances {
			if inst.ID == b.InstanceID {
				gameFile = inst.Game
				found = true
				break
			}
		}
		if found {
			// Ensure player entry exists
			s.UpdateStateAndPersist(func(st *types.ServerState) {
				if _, ok := st.Players[b.Player]; !ok {
					st.Players[b.Player] = types.Player{Name: b.Player}
				}
			})
		}
		if !found {
			http.Error(w, "instance not found", http.StatusBadRequest)
			return
		}
	}

	// If neither game nor instance provided, it's a bad request
	if gameFile == "" {
		http.Error(w, "missing game or instance_id", http.StatusBadRequest)
		return
	}

	// Let the mode handler update server state appropriately for this player-level swap
	handler := s.GetGameModeHandler()
	if err := handler.HandlePlayerSwap(b.Player, gameFile, b.InstanceID); err != nil {
		http.Error(w, "handler: "+err.Error(), http.StatusBadRequest)
		return
	}

	_ = s.SnapshotState()
}

// apiRemovePlayer: POST {player: ...}
func (s *Server) apiRemovePlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Player string `json:"player"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if b.Player == "" {
		http.Error(w, "missing player", http.StatusBadRequest)
		return
	}
	s.UpdateStateAndPersist(func(st *types.ServerState) {
		delete(st.Players, b.Player)
		if cl, ok := s.players[b.Player]; ok {
			for c, client := range s.conns {
				if client == cl {
					delete(s.conns, c)
					_ = c.Close()
					break
				}
			}
			delete(s.players, b.Player)
		}
	})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiSwapAllToGame: POST {game:...}
func (s *Server) apiSwapAllToGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Game string `json:"game"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	var players []string
	s.UpdateStateAndPersist(func(st *types.ServerState) {
		for name, player := range st.Players {
			players = append(players, name)
			player.Game = b.Game
			st.Players[name] = player
		}
	})
	results := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range players {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			cmdID := fmt.Sprintf("swap-%d-%s", time.Now().UnixNano(), p)
			cmd := types.Command{
				Cmd:     types.CmdSwap,
				ID:      cmdID,
				Payload: map[string]string{"game": b.Game},
			}
			res, err := s.sendAndWait(p, cmd, 20*time.Second)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if errors.Is(err, ErrTimeout) {
					results[p] = "timeout"
				} else {
					results[p] = "send_failed: " + err.Error()
				}
			} else {
				results[p] = res
			}
		}(p)
	}

	wg.Wait()
	if err := json.NewEncoder(w).Encode(results); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}
