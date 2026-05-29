package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/clienthost"
)

// TestWSClientAgainstServer exercises clienthost.WSClient against a live server without BizHawk.
func TestWSClientAgainstServer(t *testing.T) {
	ts := StartTestServer(t)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(ts.DataDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	if err := clienthost.EnsureDataDirs(ts.DataDir); err != nil {
		t.Fatal(err)
	}
	bipc, err := clienthost.NewBizhawkIPC(ts.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bipc.Close() })

	wsURL := HTTPToWS(ts.URL)
	cfg := clienthost.Config{"name": "join-ws-client"}
	api := clienthost.NewAPI(ts.URL, http.DefaultClient, cfg)
	ws := clienthost.NewWSClient(wsURL, api, bipc)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	done := make(chan struct{})
	go func() {
		ws.Start(ctx, cfg)
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timeout waiting for WS client hello")
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
	p, ok := out.State.Players["join-ws-client"]
	if !ok || !p.Connected {
		t.Fatalf("expected connected player join-ws-client, got %+v", out.State.Players)
	}

	ws.Stop()
}

// TestStartJoinSessionAgainstServer connects via StartJoinSession when play dependencies are satisfied.
func TestStartJoinSessionAgainstServer(t *testing.T) {
	ts := StartTestServer(t)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(ts.DataDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	if err := clienthost.AssertPlayReady(ts.DataDir); err != nil {
		t.Skipf("play dependencies not ready: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	session, err := clienthost.StartJoinSession(ctx, ts.DataDir, clienthost.JoinOptions{
		ServerURL:  strings.TrimSuffix(ts.URL, "/"),
		PlayerName: "join-session-player",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Stop() })

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(ts.URL + "/state.json")
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			State struct {
				Players map[string]struct {
					Connected bool `json:"connected"`
				} `json:"players"`
			} `json:"state"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			_ = res.Body.Close()
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if p, ok := out.State.Players["join-session-player"]; ok && p.Connected {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("player never connected via StartJoinSession")
}
