package serverhost

import "github.com/michael4d45/bizshuffle/protocol"

func (s *Server) snapshotAllPlayers() map[string]protocol.Player {
	snap := make(map[string]protocol.Player)
	s.withRLock(func() {
		for name, p := range s.state.Players {
			snap[name] = p
		}
	})
	return snap
}

func (s *Server) snapshotPlayers(names ...string) map[string]protocol.Player {
	snap := make(map[string]protocol.Player, len(names))
	s.withRLock(func() {
		for _, name := range names {
			if p, ok := s.state.Players[name]; ok {
				snap[name] = p
			}
		}
	})
	return snap
}

func (s *Server) restorePlayers(snap map[string]protocol.Player) {
	if len(snap) == 0 {
		return
	}
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		if st.Players == nil {
			st.Players = make(map[string]protocol.Player)
		}
		for name, p := range snap {
			st.Players[name] = p
		}
	})
}
