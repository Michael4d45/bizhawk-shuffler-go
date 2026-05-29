package serverhost

import (
	"os"
	"path/filepath"

	"github.com/michael4d45/bizshuffle/protocol"
)

// clearPendingForPlayer drops save waits for a player who cannot upload (disconnect).
// Must run inside UpdateStateAndPersist (server write lock held).
func (s *Server) clearPendingForPlayer(st *protocol.ServerState, playerName string) {
	for i, inst := range st.GameSwapInstances {
		if inst.FileState != protocol.FileStatePending || inst.PendingPlayer != playerName {
			continue
		}
		s.pendingInstancecount--
		st.GameSwapInstances[i].FileState = instanceFileStateFromDisk(inst.ID)
		st.GameSwapInstances[i].PendingPlayer = ""
	}
}

func instanceFileStateFromDisk(instanceID string) protocol.FileState {
	savePath := filepath.Join("./saves", instanceID+".state")
	if _, err := os.Stat(savePath); err == nil {
		return protocol.FileStateReady
	}
	return protocol.FileStateNone
}

// clearPendingInstance clears a single pending instance (e.g. owner offline during RequestPendingSaves).
func (s *Server) clearPendingInstance(instanceID string) {
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		for i, inst := range st.GameSwapInstances {
			if inst.ID != instanceID || inst.FileState != protocol.FileStatePending {
				continue
			}
			s.pendingInstancecount--
			st.GameSwapInstances[i].FileState = instanceFileStateFromDisk(instanceID)
			st.GameSwapInstances[i].PendingPlayer = ""
			return
		}
	})
}
