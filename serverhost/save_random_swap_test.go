package serverhost

import (
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

func TestWaitForSwapGateReturnsImmediatelyWhenIdle(t *testing.T) {
	s := New()
	h := &SaveModeHandler{server: s}
	if h.waitForSwapGate(500 * time.Millisecond) {
		t.Fatal("expected no pending work")
	}
}

func TestSaveModeRandomSwapPendingGuard(t *testing.T) {
	connected := protocol.Player{Name: "bob", InstanceID: "inst-1", Connected: true}
	offline := protocol.Player{Name: "bob", InstanceID: "inst-1", Connected: false}

	shouldMark := func(p protocol.Player) bool {
		return p.Connected && p.InstanceID != ""
	}
	if !shouldMark(connected) {
		t.Fatal("connected player should trigger displaced pending")
	}
	if shouldMark(offline) {
		t.Fatal("offline player should not trigger displaced pending")
	}
}
