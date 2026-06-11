package serverhost

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestAPIStartAndPauseToggleRunning(t *testing.T) {
	chdirToTemp(t)
	s := New()
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	for _, path := range []string{"/api/start", "/api/pause"} {
		res, err := http.Post(srv.URL+path, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s status %d body %s", path, res.StatusCode, body)
		}
	}

	st := s.SnapshotState()
	if st.Running {
		t.Fatal("expected running false after pause")
	}
}

func TestAPIResetPausesClearsSavesAndCompletions(t *testing.T) {
	chdirToTemp(t)
	s := New()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Running = true
		st.Players = map[string]protocol.Player{
			"alice": {
				Name:               "alice",
				CompletedGames:     []string{"a.zip"},
				CompletedInstances: []string{"inst-a"},
			},
		}
		st.GameSwapInstances = []protocol.GameSwapInstance{
			{ID: "inst-a", Game: "a.zip", FileState: protocol.FileStateReady},
		}
	})
	if err := os.MkdirAll("./saves", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("./saves/inst-a.state", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := http.Post(srv.URL+"/api/reset", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reset status %d body %s", res.StatusCode, body)
	}

	st := s.SnapshotState()
	if st.Running {
		t.Fatal("expected running false after reset")
	}
	if len(st.Players["alice"].CompletedGames) != 0 || len(st.Players["alice"].CompletedInstances) != 0 {
		t.Fatal("expected completions cleared")
	}
	if st.GameSwapInstances[0].FileState != protocol.FileStateNone {
		t.Fatalf("file_state %q want none", st.GameSwapInstances[0].FileState)
	}
	if _, err := os.Stat("./saves/inst-a.state"); !os.IsNotExist(err) {
		t.Fatal("expected host saves cleared")
	}
}

func TestAPIToggleSwapsFlipsFlag(t *testing.T) {
	chdirToTemp(t)
	s := New()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) { st.SwapEnabled = true })
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := http.Post(srv.URL+"/api/toggle_swaps", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if s.SnapshotState().SwapEnabled {
		t.Fatal("expected swap disabled")
	}
}
