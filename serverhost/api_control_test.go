package serverhost

import (
	"io"
	"net/http"
	"net/http/httptest"
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
