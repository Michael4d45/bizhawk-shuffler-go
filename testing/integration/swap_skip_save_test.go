package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestServerConnectSwapUsesSkipSave(t *testing.T) {
	ts := StartTestServer(t)
	base := ts.URL

	gamesBody, _ := json.Marshal(map[string]any{
		"games":      []string{"connect-test.zip"},
		"main_games": []map[string]string{{"file": "connect-test.zip"}},
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

	if err := client.Hello("joiner", true); err != nil {
		t.Fatal(err)
	}

	cmd, err := client.WaitFor(func(c protocol.Command) bool {
		return c.Cmd == protocol.CmdSwap
	}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !payloadBool(cmd.Payload, "skip_save") {
		t.Fatalf("expected skip_save on connect swap, got %+v", cmd.Payload)
	}
	if game, _ := cmd.Payload.(map[string]any)["game"].(string); game != "connect-test.zip" {
		t.Fatalf("expected game connect-test.zip, got %+v", cmd.Payload)
	}
}

func payloadBool(payload any, key string) bool {
	m, ok := payload.(map[string]any)
	if !ok {
		return false
	}
	v, ok := m[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}
