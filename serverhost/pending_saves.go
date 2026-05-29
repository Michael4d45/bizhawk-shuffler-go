package serverhost

import (
	"log"
	"time"
)

// WaitForPendingSaves polls until instance uploads finish or timeout. Returns true if still waiting.
func (s *Server) WaitForPendingSaves(timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var waitingFiles bool
		var waitingCmds bool
		s.withRLock(func() {
			waitingFiles = s.pendingInstancecount > 0
			waitingCmds = len(s.pending) > 0
		})
		if !waitingFiles && !waitingCmds {
			return false
		}
		// TS parity: requestPendingSaves() runs once before this wait loop, not on every tick.
		time.Sleep(200 * time.Millisecond)
	}
	var stillWaiting bool
	s.withRLock(func() {
		stillWaiting = s.pendingInstancecount > 0 || len(s.pending) > 0
		if stillWaiting {
			log.Printf("WaitForPendingSaves: timed out with pendingInstancecount=%d pendingCmds=%d",
				s.pendingInstancecount, len(s.pending))
		}
	})
	if stillWaiting {
		s.releaseUnresolvedPendingInstances()
	}
	return stillWaiting
}
