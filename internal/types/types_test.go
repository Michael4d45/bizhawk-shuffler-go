package types

import (
	"encoding/json"
	"testing"
)

func TestGameModeString(t *testing.T) {
	tests := []struct {
		mode     GameMode
		expected string
	}{
		{GameModeSync, "sync"},
		{GameModeSave, "save"},
		{GameModeUnknown, "unknown"},
	}

	for _, test := range tests {
		if got := test.mode.String(); got != test.expected {
			t.Errorf("GameMode(%d).String() = %q, want %q", test.mode, got, test.expected)
		}
	}
}

func TestParseGameMode(t *testing.T) {
	tests := []struct {
		input    string
		expected GameMode
	}{
		{"sync", GameModeSync},
		{"SYNC", GameModeSync},
		{"", GameModeSync},               // empty string defaults to sync
		{"  sync  ", GameModeSync},       // with whitespace
		{"save", GameModeSave},
		{"SAVE", GameModeSave},
		{"  save  ", GameModeSave},       // with whitespace
		{"invalid", GameModeUnknown},
		{"random", GameModeUnknown},
	}

	for _, test := range tests {
		if got := ParseGameMode(test.input); got != test.expected {
			t.Errorf("ParseGameMode(%q) = %d, want %d", test.input, got, test.expected)
		}
	}
}

func TestGameModeMarshalJSON(t *testing.T) {
	tests := []struct {
		mode     GameMode
		expected string
	}{
		{GameModeSync, `"sync"`},
		{GameModeSave, `"save"`},
		{GameModeUnknown, `"unknown"`},
	}

	for _, test := range tests {
		data, err := json.Marshal(test.mode)
		if err != nil {
			t.Errorf("json.Marshal(%d) error: %v", test.mode, err)
			continue
		}
		if string(data) != test.expected {
			t.Errorf("json.Marshal(%d) = %s, want %s", test.mode, data, test.expected)
		}
	}
}

func TestGameModeUnmarshalJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected GameMode
	}{
		{`"sync"`, GameModeSync},
		{`"save"`, GameModeSave},
		{`"unknown"`, GameModeUnknown},
		{`"invalid"`, GameModeUnknown},
		{`""`, GameModeSync}, // empty string defaults to sync
		{`1`, GameModeSync},  // integer fallback
		{`2`, GameModeSave},  // integer fallback
	}

	for _, test := range tests {
		var mode GameMode
		err := json.Unmarshal([]byte(test.input), &mode)
		if err != nil {
			t.Errorf("json.Unmarshal(%s) error: %v", test.input, err)
			continue
		}
		if mode != test.expected {
			t.Errorf("json.Unmarshal(%s) = %d, want %d", test.input, mode, test.expected)
		}
	}
}

func TestServerStateJSONCompatibility(t *testing.T) {
	// Test that ServerState can marshal/unmarshal properly with the new GameMode field
	state := ServerState{
		Running:     true,
		SwapEnabled: true,
		Mode:        GameModeSync,
		Games:       []string{"game1", "game2"},
		Players:     map[string]Player{},
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal(ServerState) error: %v", err)
	}

	var unmarshaled ServerState
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("json.Unmarshal(ServerState) error: %v", err)
	}

	if unmarshaled.Mode != GameModeSync {
		t.Errorf("Unmarshaled Mode = %d, want %d", unmarshaled.Mode, GameModeSync)
	}

	// Test with string input for backwards compatibility
	stringJSON := `{"running":true,"swap_enabled":true,"mode":"save","games":["game1"],"players":{}}`
	err = json.Unmarshal([]byte(stringJSON), &unmarshaled)
	if err != nil {
		t.Fatalf("json.Unmarshal(string mode) error: %v", err)
	}

	if unmarshaled.Mode != GameModeSave {
		t.Errorf("Unmarshaled Mode from string = %d, want %d", unmarshaled.Mode, GameModeSave)
	}
}