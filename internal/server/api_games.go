package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// apiGames: GET returns games, POST accepts JSON body {"games":[...]}
func (s *Server) apiGames(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		games, mainGames, gameInstances := s.SnapshotGames()
		resp := map[string]any{"main_games": mainGames, "game_instances": gameInstances, "games": games}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "failed to encode response: "+err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}
	if r.Method == http.MethodPost {
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Mutate state and persist via helper to centralize UpdatedAt + save
		s.UpdateStateAndPersist(func(st *types.ServerState) {
			if gms, ok := raw["games"]; ok {
				b, _ := json.Marshal(gms)
				var games []string
				if err := json.Unmarshal(b, &games); err == nil {
					st.Games = games
				}
			}
			if mg, ok := raw["main_games"]; ok {
				b, _ := json.Marshal(mg)
				var entries []types.GameEntry
				if err := json.Unmarshal(b, &entries); err == nil {
					st.MainGames = entries
				}
			}
			if gi, ok := raw["game_instances"]; ok {
				b, _ := json.Marshal(gi)
				var instances []types.GameSwapInstance
				if err := json.Unmarshal(b, &instances); err == nil {
					// Build a set of old instance IDs before updating
					oldInstanceIDs := make(map[string]bool)
					for _, oldInst := range st.GameSwapInstances {
						oldInstanceIDs[oldInst.ID] = true
					}

					// Initialize FileState for new instances
					for i := range instances {
						if instances[i].FileState == "" {
							instances[i].FileState = types.FileStateNone
						}
					}

					// Build a set of new instance IDs
					newInstanceIDs := make(map[string]bool)
					for _, newInst := range instances {
						newInstanceIDs[newInst.ID] = true
					}

					// Find removed instance IDs (in old but not in new)
					removedInstanceIDs := make(map[string]bool)
					for oldID := range oldInstanceIDs {
						if !newInstanceIDs[oldID] {
							removedInstanceIDs[oldID] = true
						}
					}

					// Unassign players from removed instances
					if len(removedInstanceIDs) > 0 {
						for playerName, player := range st.Players {
							if player.InstanceID != "" && removedInstanceIDs[player.InstanceID] {
								player.InstanceID = ""
								player.Game = ""
								st.Players[playerName] = player
							}
						}
					}

					st.GameSwapInstances = instances
				}
			}
		})
		s.broadcastToPlayers(types.Command{Cmd: types.CmdGamesUpdate, Payload: map[string]any{
			"game_instances": s.state.GameSwapInstances,
			"main_games":     s.state.MainGames,
			"games":          s.state.Games,
		}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
		if _, err := w.Write([]byte("ok")); err != nil {
			fmt.Printf("write response error: %v\n", err)
		}
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// apiInterval: GET/POST to view or set interval seconds
func (s *Server) apiInterval(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		var minv, maxv int
		s.withRLock(func() {
			minv = s.state.MinIntervalSecs
			maxv = s.state.MaxIntervalSecs
		})
		if err := json.NewEncoder(w).Encode(map[string]any{"min_interval_secs": minv, "max_interval_secs": maxv}); err != nil {
			fmt.Printf("encode response error: %v\n", err)
		}
		return
	}
	if r.Method == http.MethodPost {
		var b struct {
			MinInterval int `json:"min_interval_secs"`
			MaxInterval int `json:"max_interval_secs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		s.UpdateStateAndPersist(func(st *types.ServerState) {
			if b.MinInterval != 0 {
				st.MinIntervalSecs = b.MinInterval
			}
			if b.MaxInterval != 0 {
				st.MaxIntervalSecs = b.MaxInterval
			}
		})
		if _, err := w.Write([]byte("ok")); err != nil {
			fmt.Printf("write response error: %v\n", err)
		}
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// apiMarkGameCompletedForAll: POST /api/games/{game}/mark_completed_all
// Marks the specified game as completed for all players
func (s *Server) apiMarkGameCompletedForAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Parse game from path: /api/games/{game}/mark_completed_all
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 || pathParts[0] != "api" || pathParts[1] != "games" || pathParts[3] != "mark_completed_all" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	game := pathParts[2]
	if game == "" {
		http.Error(w, "missing game", http.StatusBadRequest)
		return
	}

	s.UpdateStateAndPersist(func(st *types.ServerState) {
		if st.Players == nil {
			return
		}
		for playerName, player := range st.Players {
			// Check if already in list
			found := false
			for _, cg := range player.CompletedGames {
				if cg == game {
					found = true
					break
				}
			}
			if !found {
				player.CompletedGames = append(player.CompletedGames, game)
				st.Players[playerName] = player
			}
		}
	})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiMarkInstanceCompletedForAll: POST /api/instances/{instance}/mark_completed_all
// Marks the specified instance as completed for all players
func (s *Server) apiMarkInstanceCompletedForAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Parse instance from path: /api/instances/{instance}/mark_completed_all
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 || pathParts[0] != "api" || pathParts[1] != "instances" || pathParts[3] != "mark_completed_all" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	instance := pathParts[2]
	if instance == "" {
		http.Error(w, "missing instance", http.StatusBadRequest)
		return
	}

	s.UpdateStateAndPersist(func(st *types.ServerState) {
		if st.Players == nil {
			return
		}
		for playerName, player := range st.Players {
			// Check if already in list
			found := false
			for _, ci := range player.CompletedInstances {
				if ci == instance {
					found = true
					break
				}
			}
			if !found {
				player.CompletedInstances = append(player.CompletedInstances, instance)
				st.Players[playerName] = player
			}
		}
	})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// handleGameCompletedRoutes routes game completed actions
func (s *Server) handleGameCompletedRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/games/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		// This might be a regular games request, let it fall through
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	game := parts[0]
	action := parts[1]

	// Create a new request with the original path for the API handlers to parse
	originalPath := r.URL.Path
	defer func() { r.URL.Path = originalPath }()

	if action == "mark_completed_all" && game != "" {
		s.apiMarkGameCompletedForAll(w, r)
	} else {
		http.Error(w, "invalid action", http.StatusBadRequest)
	}
}

// handleInstanceCompletedRoutes routes instance completed actions
func (s *Server) handleInstanceCompletedRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/instances/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	instance := parts[0]
	action := parts[1]

	// Create a new request with the original path for the API handlers to parse
	originalPath := r.URL.Path
	defer func() { r.URL.Path = originalPath }()

	if action == "mark_completed_all" && instance != "" {
		s.apiMarkInstanceCompletedForAll(w, r)
	} else {
		http.Error(w, "invalid action", http.StatusBadRequest)
	}
}
