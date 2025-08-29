package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// apiGames: GET returns games, POST accepts JSON body {"games":[...]}
func (s *Server) apiGames(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.Lock()
		resp := map[string]any{"main_games": s.state.MainGames, "game_instances": s.state.GameSwapInstances, "games": s.state.Games}
		s.mu.Unlock()
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
		s.mu.Lock()
		// Support legacy and new payloads:
		// - { "games": ["a","b"] } -> updates s.state.Games (sync mode list)
		// - { "main_games": [...], "game_instances": [...] } -> existing behavior
		if gms, ok := raw["games"]; ok {
			b, _ := json.Marshal(gms)
			var games []string
			if err := json.Unmarshal(b, &games); err == nil {
				s.state.Games = games
			}
		}
		if mg, ok := raw["main_games"]; ok {
			b, _ := json.Marshal(mg)
			var entries []types.GameEntry
			if err := json.Unmarshal(b, &entries); err == nil {
				s.state.MainGames = entries
			}
		}
		if gi, ok := raw["game_instances"]; ok {
			b, _ := json.Marshal(gi)
			var instances []types.GameSwapInstance
			if err := json.Unmarshal(b, &instances); err == nil {
				// Initialize FileState for new instances
				for i := range instances {
					if instances[i].FileState == "" {
						instances[i].FileState = types.FileStateNone
					}
				}
				s.state.GameSwapInstances = instances
			}
		}
		s.state.UpdatedAt = time.Now()
		s.mu.Unlock()
		if err := s.saveState(); err != nil {
			fmt.Printf("saveState error: %v\n", err)
		}
		s.broadcast(types.Command{Cmd: types.CmdStateUpdate, Payload: map[string]any{"updated_at": s.state.UpdatedAt}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
		s.broadcast(types.Command{Cmd: types.CmdGamesUpdate, Payload: map[string]any{
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
		s.mu.Lock()
		minv := s.state.MinIntervalSecs
		maxv := s.state.MaxIntervalSecs
		s.mu.Unlock()
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
		s.mu.Lock()
		if b.MinInterval != 0 {
			s.state.MinIntervalSecs = b.MinInterval
		}
		if b.MaxInterval != 0 {
			s.state.MaxIntervalSecs = b.MaxInterval
		}
		s.state.UpdatedAt = time.Now()
		s.mu.Unlock()
		if err := s.saveState(); err != nil {
			fmt.Printf("saveState error: %v\n", err)
		}
		s.broadcast(types.Command{Cmd: types.CmdStateUpdate, Payload: map[string]any{"updated_at": s.state.UpdatedAt}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
		if _, err := w.Write([]byte("ok")); err != nil {
			fmt.Printf("write response error: %v\n", err)
		}
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
