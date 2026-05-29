package domain

import (
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestServerSessionUpdate(t *testing.T) {
	s := NewServerSession(nil)
	updated := s.Update(func(st *protocol.ServerState) {
		st.Running = true
	})
	if updated == "" {
		t.Fatal("expected updated_at")
	}
	if !s.Snapshot().Running {
		t.Fatal("running not set")
	}
}
