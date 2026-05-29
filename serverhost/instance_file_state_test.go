package serverhost

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestSetInstanceFileStatePendingTracksCount(t *testing.T) {
	chdirToTemp(t)
	s := New()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.GameSwapInstances = []protocol.GameSwapInstance{{ID: "inst-a", Game: "a.zip"}}
	})

	s.setInstanceFileStateWithPlayer("inst-a", protocol.FileStatePending, "bob")
	if s.PendingInstanceCount() != 1 {
		t.Fatalf("count %d want 1", s.PendingInstanceCount())
	}
	st := s.SnapshotState()
	if st.GameSwapInstances[0].FileState != protocol.FileStatePending {
		t.Fatalf("state %q", st.GameSwapInstances[0].FileState)
	}

	s.setInstanceFileState("inst-a", protocol.FileStateNone)
	if s.PendingInstanceCount() != 0 {
		t.Fatalf("count %d after clear", s.PendingInstanceCount())
	}
}

func TestSetInstanceFileStateReadyAfterUploadPath(t *testing.T) {
	chdirToTemp(t)
	if err := os.MkdirAll("./saves", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("./saves", "inst-a.state"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.GameSwapInstances = []protocol.GameSwapInstance{{
			ID: "inst-a", Game: "a.zip", FileState: protocol.FileStatePending, PendingPlayer: "bob",
		}}
		s.pendingInstancecount = 1
	})
	s.setInstanceFileState("inst-a", protocol.FileStateReady)
	st := s.SnapshotState()
	if st.GameSwapInstances[0].FileState != protocol.FileStateReady {
		t.Fatalf("state %q", st.GameSwapInstances[0].FileState)
	}
}
