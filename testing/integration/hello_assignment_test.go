package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestHelloAssignsGameAndSendsSwapWhenReady(t *testing.T) {
	ts := StartTestServer(t)
	base := ts.URL

	modeBody, _ := json.Marshal(map[string]string{"mode": "save"})
	res, err := http.Post(base+"/api/mode", "application/json", bytes.NewReader(modeBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	gamesBody, _ := json.Marshal(map[string]any{
		"game_instances": []map[string]string{
			{"id": "inst-1", "game": "mario.zip", "file_state": "none"},
		},
	})
	res, err = http.Post(base+"/api/games", "application/json", bytes.NewReader(gamesBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	client := NewWSTestClient(base)
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.Hello("joiner", true); err != nil {
		t.Fatal(err)
	}

	cmd, err := client.WaitFor(func(c protocol.Command) bool {
		return c.Cmd == protocol.CmdSwap
	}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := cmd.Payload.(map[string]any)
	if !ok {
		t.Fatalf("swap payload type %T", cmd.Payload)
	}
	if payload["game"] != "mario.zip" {
		t.Fatalf("swap game %+v", payload)
	}
	if payload["instance_id"] != "inst-1" {
		t.Fatalf("swap instance %+v", payload)
	}

	stRes, err := http.Get(base + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stRes.Body.Close() }()
	var out struct {
		State struct {
			Players map[string]struct {
				Game       string `json:"game"`
				InstanceID string `json:"instance_id"`
			} `json:"players"`
		} `json:"state"`
	}
	if err := json.NewDecoder(stRes.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.State.Players["joiner"].Game != "mario.zip" {
		t.Fatalf("player state %+v", out.State.Players["joiner"])
	}
}

func TestHelloDefersSwapUntilBizhawkReady(t *testing.T) {
	ts := StartTestServer(t)
	base := ts.URL

	gamesBody, _ := json.Marshal(map[string]any{
		"games":      []string{"test.zip"},
		"main_games": []map[string]string{{"file": "test.zip"}},
	})
	res, err := http.Post(base+"/api/games", "application/json", bytes.NewReader(gamesBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	client := NewWSTestClient(base)
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.Hello("late-ready", false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	client.mu.Lock()
	swapCount := 0
	for _, cmd := range client.inbox {
		if cmd.Cmd == protocol.CmdSwap {
			swapCount++
		}
	}
	client.mu.Unlock()
	if swapCount != 0 {
		t.Fatalf("expected no swap before ready, got %d", swapCount)
	}

	if err := client.Send(protocol.Command{
		Cmd:     protocol.CmdStatusUpdate,
		ID:      "status-1",
		Payload: map[string]any{"bizhawk_ready": true},
	}); err != nil {
		t.Fatal(err)
	}

	_, err = client.WaitFor(func(c protocol.Command) bool {
		return c.Cmd == protocol.CmdSwap && c.Payload != nil
	}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
}
