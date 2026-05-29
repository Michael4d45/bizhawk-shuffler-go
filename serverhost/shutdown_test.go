package serverhost

import (
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestShutdownWithNoActiveWebsockets(t *testing.T) {
	chdirToTemp(t)
	s := New()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Running = true
	})
	s.BeginShutdown()
	s.Shutdown()
	st := s.SnapshotState()
	if st.Running {
		t.Fatal("expected running=false after shutdown")
	}
}
