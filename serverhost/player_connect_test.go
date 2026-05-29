package serverhost

import (
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestAssignPlayerOnConnectSyncMode(t *testing.T) {
	s := New()
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Games = []string{"mario.zip"}
		st.Players["joiner"] = protocol.Player{Name: "joiner"}
	})

	p := s.AssignPlayerOnConnect("joiner")
	if p.Game != "mario.zip" {
		t.Fatalf("game %q", p.Game)
	}
	st := s.SnapshotState()
	if st.Players["joiner"].Game != "mario.zip" {
		t.Fatalf("persisted game %q", st.Players["joiner"].Game)
	}
}

func TestShouldSendSwapDedupes(t *testing.T) {
	s := New()
	p := protocol.Player{Name: "p1", Game: "a.zip"}
	s.recordSwapApplied("p1", p)
	if s.ShouldSendSwap(p, false) {
		t.Fatal("expected duplicate swap suppressed")
	}
	if !s.ShouldSendSwap(p, true) {
		t.Fatal("expected force swap")
	}
}
