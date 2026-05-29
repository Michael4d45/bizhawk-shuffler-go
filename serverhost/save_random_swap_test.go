package serverhost

import (
	"testing"
	"time"
)

func TestWaitForSwapGateReturnsImmediatelyWhenIdle(t *testing.T) {
	s := New()
	h := &SaveModeHandler{server: s}
	if h.waitForSwapGate(500 * time.Millisecond) {
		t.Fatal("expected no pending work")
	}
}

