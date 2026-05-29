package serverhost

import "github.com/michael4d45/bizshuffle/protocol"

// AssignPlayerOnConnect persists game-mode assignment for a newly connected player.
func (s *Server) AssignPlayerOnConnect(name string) protocol.Player {
	assigned := s.currentPlayer(name)
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		if st.Players == nil {
			st.Players = make(map[string]protocol.Player)
		}
		p, ok := st.Players[name]
		if !ok {
			p = protocol.Player{Name: name}
		}
		changed := false
		if assigned.Game != "" && p.Game == "" {
			p.Game = assigned.Game
			changed = true
		}
		if assigned.InstanceID != "" && p.InstanceID == "" {
			p.InstanceID = assigned.InstanceID
			changed = true
		}
		if changed || !ok {
			st.Players[name] = p
		}
	})
	return s.currentPlayer(name)
}

func (s *Server) swapTargetKey(player protocol.Player) string {
	return player.Game + "\x00" + player.InstanceID
}

// ShouldSendSwap reports whether the client still needs a swap for this assignment.
func (s *Server) ShouldSendSwap(player protocol.Player, force bool) bool {
	if force {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.appliedSwapTarget == nil {
		return true
	}
	return s.appliedSwapTarget[player.Name] != s.swapTargetKey(player)
}

func (s *Server) recordSwapApplied(playerName string, player protocol.Player) {
	s.withLock(func() {
		if s.appliedSwapTarget == nil {
			s.appliedSwapTarget = make(map[string]string)
		}
		s.appliedSwapTarget[playerName] = s.swapTargetKey(player)
	})
}

// ClearAppliedSwap forgets the last swap target for a player (e.g. on disconnect).
func (s *Server) ClearAppliedSwap(playerName string) {
	if playerName == "" {
		return
	}
	s.withLock(func() {
		delete(s.appliedSwapTarget, playerName)
	})
}

// SwapSendOptions configures an outbound swap command.
type SwapSendOptions struct {
	SkipSave bool
	Force    bool
}
