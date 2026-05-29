package serverhost

import (
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

// WaitForPendingSaves must not re-send request_save on every poll (TS only sends once before waiting).
func TestWaitForPendingSavesDoesNotResendRequestSave(t *testing.T) {
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

	before := s.PendingCommandCount()
	// Short timeout: loop should sleep, not call RequestPendingSaves.
	done := make(chan bool, 1)
	go func() {
		done <- s.WaitForPendingSaves(500 * time.Millisecond)
	}()
	<-done
	after := s.PendingCommandCount()
	if after > before+1 {
		t.Fatalf("pending commands grew by %d during wait (before=%d after=%d); expected no repeated request_save", after-before, before, after)
	}
}
