package clienthost

import (
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

// Pending swap must be taken out before Handle runs so a failed replay does not loop forever.
func TestPendingSwapTakenBeforeHandle(t *testing.T) {
	ctrl := &Controller{}
	ctrl.pendingSwap = protocol.Command{Cmd: protocol.CmdSwap, ID: "swap-1"}

	ctrl.mu.Lock()
	pending := ctrl.pendingSwap
	ctrl.pendingSwap = protocol.Command{}
	ctrl.mu.Unlock()

	if pending.Cmd != protocol.CmdSwap {
		t.Fatalf("pending cmd %q", pending.Cmd)
	}
	if ctrl.pendingSwap.Cmd != "" {
		t.Fatal("slot should be empty after take")
	}
}
