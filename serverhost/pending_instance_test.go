package serverhost

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestClearPendingForPlayerOnDisconnect(t *testing.T) {
	s := New()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.GameSwapInstances = []protocol.GameSwapInstance{{
			ID:            "inst-a",
			Game:          "a.zip",
			FileState:     protocol.FileStatePending,
			PendingPlayer: "bob",
		}}
		st.Players["bob"] = protocol.Player{Name: "bob", Connected: true}
		s.pendingInstancecount = 1
	})

	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		pl := st.Players["bob"]
		pl.Connected = false
		pl.BizhawkReady = false
		st.Players["bob"] = pl
		s.clearPendingForPlayer(st, "bob")
	})

	st := s.SnapshotState()
	inst := st.GameSwapInstances[0]
	if inst.FileState != protocol.FileStateNone {
		t.Fatalf("file_state %q want none", inst.FileState)
	}
	if inst.PendingPlayer != "" {
		t.Fatalf("pending_player %q", inst.PendingPlayer)
	}
	if s.PendingInstanceCount() != 0 {
		t.Fatalf("pendingInstancecount %d", s.PendingInstanceCount())
	}
}

func TestClearPendingForPlayerKeepsReadyWhenSaveOnDisk(t *testing.T) {
	s := New()
	if err := os.MkdirAll("./saves", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll("./saves") })
	if err := os.WriteFile(filepath.Join("./saves", "inst-a.state"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.GameSwapInstances = []protocol.GameSwapInstance{{
			ID:            "inst-a",
			Game:          "a.zip",
			FileState:     protocol.FileStatePending,
			PendingPlayer: "bob",
		}}
		s.pendingInstancecount = 1
	})
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		s.clearPendingForPlayer(st, "bob")
	})

	if st := s.SnapshotState(); st.GameSwapInstances[0].FileState != protocol.FileStateReady {
		t.Fatalf("file_state %q want ready", st.GameSwapInstances[0].FileState)
	}
}

func TestRequestPendingSavesSkipsDisconnected(t *testing.T) {
	s := New()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.GameSwapInstances = []protocol.GameSwapInstance{{
			ID:            "inst-a",
			Game:          "a.zip",
			FileState:     protocol.FileStatePending,
			PendingPlayer: "bob",
		}}
		st.Players["bob"] = protocol.Player{Name: "bob", Connected: false}
		s.pendingInstancecount = 1
	})

	s.RequestPendingSaves()

	st := s.SnapshotState()
	if st.GameSwapInstances[0].FileState == protocol.FileStatePending {
		t.Fatal("expected pending cleared for offline player")
	}
	if st.GameSwapInstances[0].PendingPlayer != "" {
		t.Fatalf("pending_player %q", st.GameSwapInstances[0].PendingPlayer)
	}
}
