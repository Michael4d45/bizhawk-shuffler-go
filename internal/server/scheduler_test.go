package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michael4d45/bizshuffle/internal/types"
)

func TestPerformSwapModeDispatch(t *testing.T) {
	tests := []struct {
		name        string
		mode        types.GameMode
		shouldError bool
	}{
		{
			name:        "sync mode",
			mode:        types.GameModeSync,
			shouldError: false,
		},
		{
			name:        "save mode",
			mode:        types.GameModeSave,
			shouldError: false,
		},
		{
			name:        "unknown mode",
			mode:        types.GameModeUnknown,
			shouldError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{
				state: types.ServerState{
					Mode:    test.mode,
					Games:   []string{"game1", "game2"},
					Players: map[string]types.Player{},
				},
				conns:   make(map[*websocket.Conn]*wsClient),
				players: make(map[string]*wsClient),
			}

			_, err := server.performSwap()
			if test.shouldError && err == nil {
				t.Errorf("Expected error for mode %s, but got none", test.mode.String())
			}
			if !test.shouldError && err != nil {
				// These will fail due to no players/connections, but should not fail due to mode dispatch
				if err.Error() != "need players and games configured" {
					t.Errorf("Unexpected error for mode %s: %v", test.mode.String(), err)
				}
			}
		})
	}
}

func TestPerformSwapSyncMode(t *testing.T) {
	// Create a mock server with players but no websocket connections
	server := &Server{
		state: types.ServerState{
			Mode:  types.GameModeSync,
			Games: []string{"game1", "game2", "game3"},
			Players: map[string]types.Player{
				"player1": {Name: "player1"},
				"player2": {Name: "player2"},
			},
		},
		conns:     make(map[*websocket.Conn]*wsClient),
		players:   make(map[string]*wsClient),
		pending:   make(map[string]chan string),
		ephemeral: make(map[string]string),
	}

	// This will fail because there are no actual websocket connections,
	// but we can test the mode-specific logic up to that point
	outcome, err := server.performSwapSync()

	// Should return an error because players aren't connected, but not due to mode logic
	if err != nil {
		t.Logf("Expected error due to no connections: %v", err)
	}

	// Even with error, we can check that the mapping was created correctly for sync mode
	if outcome != nil {
		// In sync mode, all players should get the same game
		if len(outcome.Mapping) > 0 {
			firstGame := ""
			for _, game := range outcome.Mapping {
				if firstGame == "" {
					firstGame = game
				} else if game != firstGame {
					t.Errorf("In sync mode, all players should get same game, but got different games: %v", outcome.Mapping)
				}
			}
		}
	}
}

func TestPerformSwapSaveMode(t *testing.T) {
	// Create a mock server with players but no websocket connections
	server := &Server{
		state: types.ServerState{
			Mode:  types.GameModeSave,
			Games: []string{"game1", "game2", "game3"},
			Players: map[string]types.Player{
				"player1": {Name: "player1"},
				"player2": {Name: "player2"},
				"player3": {Name: "player3"},
			},
		},
		conns:     make(map[*websocket.Conn]*wsClient),
		players:   make(map[string]*wsClient),
		pending:   make(map[string]chan string),
		ephemeral: make(map[string]string),
	}

	// This will fail because there are no actual websocket connections,
	// but we can test the mode-specific logic up to that point
	outcome, err := server.performSwapSave()

	// Should return an error because players aren't connected, but not due to mode logic
	if err != nil {
		t.Logf("Expected error due to no connections: %v", err)
	}

	// Even with error, we can check that the mapping was created correctly for save mode
	if outcome != nil {
		// In save mode, players should get different games (round-robin distribution)
		if len(outcome.Mapping) > 0 {
			games := make(map[string]bool)
			for _, game := range outcome.Mapping {
				games[game] = true
			}
			// With 3 players and 3 games, we should have all 3 games assigned
			if len(games) != 3 {
				t.Errorf("In save mode with 3 players and 3 games, expected 3 different games, got %d: %v", len(games), games)
			}
		}
	}
}

func TestCurrentGameForPlayerModeLogic(t *testing.T) {
	tests := []struct {
		name     string
		mode     types.GameMode
		player   string
		current  string
		games    []string
		expected string
	}{
		{
			name:     "sync mode with current set",
			mode:     types.GameModeSync,
			player:   "player1",
			current:  "currentgame",
			games:    []string{"game1", "game2"},
			expected: "currentgame",
		},
		{
			name:     "sync mode without current, falls back to first game",
			mode:     types.GameModeSync,
			player:   "player1",
			current:  "",
			games:    []string{"game1", "game2"},
			expected: "game1",
		},
		{
			name:     "save mode with current set",
			mode:     types.GameModeSave,
			player:   "player1",
			current:  "currentgame",
			games:    []string{"game1", "game2"},
			expected: "currentgame",
		},
		{
			name:     "save mode without current, uses first game assignment",
			mode:     types.GameModeSave,
			player:   "player1",
			current:  "",
			games:    []string{"game1", "game2"},
			expected: "game1", // Now uses first game instead of hash-based assignment
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{
				state: types.ServerState{
					Mode:  test.mode,
					Games: test.games,
					Players: map[string]types.Player{
						test.player: {
							Name:    test.player,
							Current: test.current,
						},
					},
				},
			}

			result := server.currentGameForPlayer(test.player)
			if result != test.expected {
				t.Errorf("currentGameForPlayer(%s) = %s, want %s", test.player, result, test.expected)
			}
		})
	}
}

func TestGameModeValidation(t *testing.T) {
	// Test that the server properly validates game modes in API calls
	// Test valid mode changes
	validModes := []string{"sync", "save"}
	for _, mode := range validModes {
		parsed := types.ParseGameMode(mode)
		if parsed == types.GameModeUnknown {
			t.Errorf("Valid mode %s parsed as unknown", mode)
		}
	}

	// Test invalid mode
	invalid := types.ParseGameMode("invalid_mode")
	if invalid != types.GameModeUnknown {
		t.Errorf("Invalid mode should parse as unknown, got %s", invalid.String())
	}
}

func TestGameModeJSONSerialization(t *testing.T) {
	// Test that ServerState with GameMode can be properly serialized/deserialized
	original := types.ServerState{
		Running:     true,
		SwapEnabled: true,
		Mode:        types.GameModeSave,
		Games:       []string{"game1", "game2"},
		Players:     map[string]types.Player{},
		UpdatedAt:   time.Now().UTC().Truncate(time.Second), // Truncate for comparison
	}

	// Serialize to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal ServerState: %v", err)
	}

	// Deserialize from JSON
	var restored types.ServerState
	err = json.Unmarshal(data, &restored)
	if err != nil {
		t.Fatalf("Failed to unmarshal ServerState: %v", err)
	}

	// Check that mode was preserved
	if restored.Mode != original.Mode {
		t.Errorf("Mode not preserved: got %s, want %s", restored.Mode.String(), original.Mode.String())
	}

	// Test backwards compatibility with string mode in JSON
	stringModeJSON := `{"running":true,"swap_enabled":true,"mode":"sync","games":["game1"],"players":{}}`
	err = json.Unmarshal([]byte(stringModeJSON), &restored)
	if err != nil {
		t.Fatalf("Failed to unmarshal string mode JSON: %v", err)
	}

	if restored.Mode != types.GameModeSync {
		t.Errorf("String mode not properly parsed: got %s, want sync", restored.Mode.String())
	}
}
