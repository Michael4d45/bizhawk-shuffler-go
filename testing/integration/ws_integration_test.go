package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestWebSocketHelloAdmin(t *testing.T) {
	ts := StartTestServer(t)

	client := NewWSTestClient(ts.URL)
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.HelloAdmin("test-admin"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
}

func TestWebSocketHelloPlayer(t *testing.T) {
	ts := StartTestServer(t)

	client := NewWSTestClient(ts.URL)
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.Hello("test-player", true); err != nil {
		t.Fatal(err)
	}
	_, err := client.WaitFor(func(cmd protocol.Command) bool {
		return cmd.Cmd == protocol.CmdGamesUpdate ||
			cmd.Cmd == protocol.CmdSwap ||
			cmd.Cmd == protocol.CmdResume ||
			cmd.Cmd == protocol.CmdPause
	}, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(ts.URL + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out struct {
		State struct {
			Players map[string]struct {
				Connected bool `json:"connected"`
			} `json:"players"`
		} `json:"state"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	p, ok := out.State.Players["test-player"]
	if !ok || !p.Connected {
		t.Fatalf("player not connected: %+v", out.State.Players)
	}
}
