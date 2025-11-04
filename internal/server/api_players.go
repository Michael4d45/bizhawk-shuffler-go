package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
		if cl, ok := s.playerClients[b.Player]; ok {
			for c, client := range s.conns {
				if client == cl {
					delete(s.conns, c)
					_ = c.Close()
					break
				}
			}
			delete(s.playerClients, b.Player)
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
	s.sendSwapAll()
	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiAddPlayer: POST {player:...}
// Creates a new player that hasn't connected yet (connected=false)
func (s *Server) apiAddPlayer(w http.ResponseWriter, r *http.Request) {
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
		// Initialize Players map if nil
		if st.Players == nil {
			st.Players = make(map[string]types.Player)
		}
		// Check if player already exists
		if _, ok := st.Players[b.Player]; ok {
			// Player already exists, return success (idempotent)
			return
		}
		// Create new player with connected=false
		st.Players[b.Player] = types.Player{
			Name:      b.Player,
			Connected: false,
			HasFiles:  false,
		}
	})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiAddCompletedGame: POST /api/players/{player}/completed_games with body {game: "..."}
func (s *Server) apiAddCompletedGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Parse player from path: /api/players/{player}/completed_games
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 || pathParts[0] != "api" || pathParts[1] != "players" || pathParts[3] != "completed_games" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	playerName := pathParts[2]

	var b struct {
		Game string `json:"game"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if b.Game == "" {
		http.Error(w, "missing game", http.StatusBadRequest)
		return
	}

	s.UpdateStateAndPersist(func(st *types.ServerState) {
		if st.Players == nil {
			st.Players = make(map[string]types.Player)
		}
		p, ok := st.Players[playerName]
		if !ok {
			p = types.Player{Name: playerName}
		}
		// Check if already in list
		for _, cg := range p.CompletedGames {
			if cg == b.Game {
				return // Already completed
			}
		}
		p.CompletedGames = append(p.CompletedGames, b.Game)
		st.Players[playerName] = p
	})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiRemoveCompletedGame: DELETE /api/players/{player}/completed_games?game={game}
func (s *Server) apiRemoveCompletedGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Parse player from path: /api/players/{player}/completed_games
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 || pathParts[0] != "api" || pathParts[1] != "players" || pathParts[3] != "completed_games" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	playerName := pathParts[2]

	game := r.URL.Query().Get("game")
	if game == "" {
		http.Error(w, "missing game parameter", http.StatusBadRequest)
		return
	}

	s.UpdateStateAndPersist(func(st *types.ServerState) {
		if p, ok := st.Players[playerName]; ok {
			var newList []string
			for _, cg := range p.CompletedGames {
				if cg != game {
					newList = append(newList, cg)
				}
			}
			p.CompletedGames = newList
			st.Players[playerName] = p
		}
	})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiAddCompletedInstance: POST /api/players/{player}/completed_instances with body {instance: "..."}
func (s *Server) apiAddCompletedInstance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Parse player from path: /api/players/{player}/completed_instances
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 || pathParts[0] != "api" || pathParts[1] != "players" || pathParts[3] != "completed_instances" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	playerName := pathParts[2]

	var b struct {
		Instance string `json:"instance"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if b.Instance == "" {
		http.Error(w, "missing instance", http.StatusBadRequest)
		return
	}

	s.UpdateStateAndPersist(func(st *types.ServerState) {
		if st.Players == nil {
			st.Players = make(map[string]types.Player)
		}
		p, ok := st.Players[playerName]
		if !ok {
			p = types.Player{Name: playerName}
		}
		// Check if already in list
		for _, ci := range p.CompletedInstances {
			if ci == b.Instance {
				return // Already completed
			}
		}
		p.CompletedInstances = append(p.CompletedInstances, b.Instance)
		st.Players[playerName] = p
	})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// apiRemoveCompletedInstance: DELETE /api/players/{player}/completed_instances?instance={instance}
func (s *Server) apiRemoveCompletedInstance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Parse player from path: /api/players/{player}/completed_instances
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 || pathParts[0] != "api" || pathParts[1] != "players" || pathParts[3] != "completed_instances" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	playerName := pathParts[2]

	instance := r.URL.Query().Get("instance")
	if instance == "" {
		http.Error(w, "missing instance parameter", http.StatusBadRequest)
		return
	}

	s.UpdateStateAndPersist(func(st *types.ServerState) {
		if p, ok := st.Players[playerName]; ok {
			var newList []string
			for _, ci := range p.CompletedInstances {
				if ci != instance {
					newList = append(newList, ci)
				}
			}
			p.CompletedInstances = newList
			st.Players[playerName] = p
		}
	})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok"}); err != nil {
		fmt.Printf("encode response error: %v\n", err)
	}
}

// handlePlayerCompletedRoutes routes player completed games/instances actions
func (s *Server) handlePlayerCompletedRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/players/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	action := parts[1]

	// Create a new request with the original path for the API handlers to parse
	originalPath := r.URL.Path
	defer func() { r.URL.Path = originalPath }()

	switch action {
	case "completed_games":
		switch r.Method {
		case http.MethodPost:
			s.apiAddCompletedGame(w, r)
		case http.MethodDelete:
			s.apiRemoveCompletedGame(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case "completed_instances":
		switch r.Method {
		case http.MethodPost:
			s.apiAddCompletedInstance(w, r)
		case http.MethodDelete:
			s.apiRemoveCompletedInstance(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
	}
}
