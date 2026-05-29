package serverhost

// PendingCommandCount returns in-flight WS commands awaiting client ack (tests).
func (s *Server) PendingCommandCount() int {
	var n int
	s.withRLock(func() { n = len(s.pending) })
	return n
}

// PendingInstanceCount returns instances waiting on save upload (tests).
func (s *Server) PendingInstanceCount() int {
	var n int
	s.withRLock(func() { n = s.pendingInstancecount })
	return n
}
