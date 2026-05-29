package serverhost

import (
	"context"
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

func TestStopBroadcasterIdempotent(t *testing.T) {
	chdirToTemp(t)
	s := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.StartBroadcaster(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.StopBroadcaster(); err != nil {
		t.Fatal(err)
	}
	if err := s.StopBroadcaster(); err != nil {
		t.Fatal("second stop should be no-op")
	}
}
