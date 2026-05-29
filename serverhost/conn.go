package serverhost

// connMu protects websocket client maps only (conns, playerClients, adminClients).
// Game/state data uses s.mu. Shutdown closes sockets via liveConns without either lock.
func (s *Server) withConnLock(fn func()) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	fn()
}

func (s *Server) withConnRLock(fn func()) {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	fn()
}

func (s *Server) findPlayerNameForClientLocked(client *wsClient) string {
	for n, pc := range s.playerClients {
		if pc == client {
			return n
		}
	}
	return ""
}

func (s *Server) findAdminNameForClientLocked(client *wsClient) string {
	for n, ac := range s.adminClients {
		if ac == client {
			return n
		}
	}
	return ""
}
