package server

import (
	"testing"

	"github.com/gorilla/websocket"
	"github.com/michael4d45/bizshuffle/internal/types"
)

func TestGameModeHandlers(t *testing.T) {
	tests := []struct {
		mode        types.GameMode
		expectError bool
	}{
		{types.GameModeSync, false},
		{types.GameModeSave, false},
		{types.GameModeUnknown, true},
	}

	for _, test := range tests {
		t.Run(test.mode.String(), func(t *testing.T) {
			handler, err := getGameModeHandler(test.mode)
			if test.expectError {
				if err == nil {
					t.Errorf("Expected error for mode %s, but got none", test.mode.String())
				}
				if handler != nil {
					t.Errorf("Expected nil handler for unknown mode, but got %T", handler)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for mode %s: %v", test.mode.String(), err)
				}
				if handler == nil {
					t.Errorf("Expected handler for mode %s, but got nil", test.mode.String())
				}
			}
		})
	}
}

func TestSyncModeHandler(t *testing.T) {
	handler := &SyncModeHandler{}
	
	// Test description
	desc := handler.Description()
	if desc == "" {
		t.Error("SyncModeHandler.Description() should not be empty")
	}
	if desc != "Sync Mode: All players play the same game and swap simultaneously. No save files are uploaded or downloaded during swaps." {
		t.Errorf("Unexpected description: %s", desc)
	}

	// Test GetCurrentGameForPlayer with no current games
	server := &Server{
		state: types.ServerState{
			Players: map[string]types.Player{
				"player1": {Name: "player1", Current: ""},
			},
			Games: []string{},
		},
	}
	result := handler.GetCurrentGameForPlayer(server, "player1")
	if result != "" {
		t.Errorf("Expected empty result with no games, got %s", result)
	}

	// Test GetCurrentGameForPlayer with games but no current
	server.state.Games = []string{"game1", "game2"}
	result = handler.GetCurrentGameForPlayer(server, "player1")
	if result != "game1" {
		t.Errorf("Expected first game 'game1', got %s", result)
	}

	// Test GetCurrentGameForPlayer with current set
	server.state.Players["player1"] = types.Player{Name: "player1", Current: "currentgame"}
	// Since player1 now has current set, sync mode should use that first
	result = handler.GetCurrentGameForPlayer(server, "player1")
	if result != "currentgame" {
		t.Errorf("Expected player's own current 'currentgame', got %s", result)
	}

	// Test with another player having current set, but our player has no current
	server.state.Players["player1"] = types.Player{Name: "player1", Current: ""}
	server.state.Players["player2"] = types.Player{Name: "player2", Current: "player2game"}
	result = handler.GetCurrentGameForPlayer(server, "player1")
	if result != "player2game" {
		t.Errorf("Expected 'player2game' from first player with current set, got %s", result)
	}
}

func TestSaveModeHandler(t *testing.T) {
	handler := &SaveModeHandler{}
	
	// Test description
	desc := handler.Description()
	if desc == "" {
		t.Error("SaveModeHandler.Description() should not be empty")
	}
	if desc != "Save Mode: Players play different games and perform save upload/download orchestration on swap. Each player gets a different game assigned based on a hash of their name." {
		t.Errorf("Unexpected description: %s", desc)
	}

	// Test GetCurrentGameForPlayer with no games
	server := &Server{
		state: types.ServerState{
			Players: map[string]types.Player{
				"player1": {Name: "player1", Current: ""},
			},
			Games: []string{},
		},
	}
	result := handler.GetCurrentGameForPlayer(server, "player1")
	if result != "" {
		t.Errorf("Expected empty result with no games, got %s", result)
	}

	// Test GetCurrentGameForPlayer with games
	server.state.Games = []string{"game1", "game2"}
	result1 := handler.GetCurrentGameForPlayer(server, "player1")
	result2 := handler.GetCurrentGameForPlayer(server, "player2")
	
	// Results should be deterministic based on player name hash
	if result1 == "" {
		t.Error("Expected non-empty result for player1")
	}
	if result2 == "" {
		t.Error("Expected non-empty result for player2")
	}
	
	// Same player should always get same game
	result1Again := handler.GetCurrentGameForPlayer(server, "player1")
	if result1 != result1Again {
		t.Errorf("Expected deterministic result for player1: %s != %s", result1, result1Again)
	}

	// Different players might get different games (depending on hash)
	// We can't guarantee they're different, but we can check they're both valid
	validGames := map[string]bool{"game1": true, "game2": true}
	if !validGames[result1] {
		t.Errorf("Invalid game assignment for player1: %s", result1)
	}
	if !validGames[result2] {
		t.Errorf("Invalid game assignment for player2: %s", result2)
	}
}

func TestGetAllGameModes(t *testing.T) {
	modes := GetAllGameModes()
	
	// Should have entries for all known modes
	if len(modes) != 2 {
		t.Errorf("Expected 2 game modes, got %d", len(modes))
	}

	// Check that sync mode is included
	syncDesc, ok := modes[types.GameModeSync]
	if !ok {
		t.Error("Sync mode should be included in GetAllGameModes()")
	}
	if syncDesc == "" {
		t.Error("Sync mode description should not be empty")
	}

	// Check that save mode is included
	saveDesc, ok := modes[types.GameModeSave]
	if !ok {
		t.Error("Save mode should be included in GetAllGameModes()")
	}
	if saveDesc == "" {
		t.Error("Save mode description should not be empty")
	}

	// Descriptions should be different
	if syncDesc == saveDesc {
		t.Error("Game mode descriptions should be different")
	}
}

func TestGameModeHandlerIntegration(t *testing.T) {
	// Test that the abstracted performSwap works the same as before
	server := &Server{
		state: types.ServerState{
			Mode:  types.GameModeSync,
			Games: []string{"game1", "game2"},
			Players: map[string]types.Player{
				"player1": {Name: "player1"},
			},
		},
		conns:     make(map[*websocket.Conn]*wsClient),
		players:   make(map[string]*wsClient),
		pending:   make(map[string]chan string),
		ephemeral: make(map[string]string),
	}

	// This should work without errors (even though it will fail due to no connections)
	_, err := server.performSwap()
	if err != nil && err.Error() != "need players and games configured" {
		t.Errorf("Unexpected error from abstracted performSwap: %v", err)
	}

	// Test with unknown mode
	server.state.Mode = types.GameModeUnknown
	_, err = server.performSwap()
	if err == nil {
		t.Error("Expected error for unknown game mode")
	}
}

func TestCurrentGameForPlayerAbstraction(t *testing.T) {
	// Test that the abstracted currentGameForPlayer works correctly
	server := &Server{
		state: types.ServerState{
			Mode:  types.GameModeSync,
			Games: []string{"game1", "game2"},
			Players: map[string]types.Player{
				"player1": {Name: "player1", Current: ""},
				"player2": {Name: "player2", Current: "explicit"},
			},
		},
	}

	// Player with explicit current should get that
	result := server.currentGameForPlayer("player2")
	if result != "explicit" {
		t.Errorf("Expected 'explicit' for player2, got %s", result)
	}

	// Player without current should delegate to mode handler
	result = server.currentGameForPlayer("player1")
	// In sync mode, since player2 has current "explicit", that should be used
	if result != "explicit" {
		t.Errorf("Expected 'explicit' for player1 in sync mode (from player2's current), got %s", result)
	}

	// Test with save mode - clear all current settings first
	server.state.Mode = types.GameModeSave
	server.state.Players["player1"] = types.Player{Name: "player1", Current: ""}
	server.state.Players["player2"] = types.Player{Name: "player2", Current: ""}
	result = server.currentGameForPlayer("player1")
	// Should get a deterministic game based on hash
	if result == "" {
		t.Error("Expected non-empty result for player1 in save mode")
	}

	// Test with unknown mode (should fallback to sync behavior)
	server.state.Mode = types.GameModeUnknown
	result = server.currentGameForPlayer("player1")
	if result != "game1" {
		t.Errorf("Expected fallback to sync behavior for unknown mode, got %s", result)
	}
}