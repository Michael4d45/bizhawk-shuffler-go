package serverhost

import (
	"log"
	"sync/atomic"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
)

const wsHandlerDrainWait = 3 * time.Second

// BeginShutdown marks the server as stopping so websocket teardown avoids contending on s.mu.
// Call before cancelling HTTP request contexts or closing listeners.
func (s *Server) BeginShutdown() {
	if s == nil {
		return
	}
	atomic.StoreInt32(&s.shuttingDown, 1)
	select {
	case s.schedulerCh <- struct{}{}:
	default:
	}
}

// Shutdown stops background work and waits for websocket handlers to exit.
// Call this after cancelling the HTTP server's request context (BaseContext).
func (s *Server) Shutdown() {
	if s == nil {
		return
	}
	s.BeginShutdown()

	log.Printf("serverhost: closing websocket clients")
	s.closeWebSocketsBounded(closeWebSocketsWait)

	handlerDone := make(chan struct{})
	go func() {
		s.wsActive.Wait()
		close(handlerDone)
	}()
	select {
	case <-handlerDone:
		log.Printf("serverhost: websocket handlers exited")
	case <-time.After(wsHandlerDrainWait):
		log.Printf("serverhost: websocket handler drain timeout; forcing close")
		s.closeWebSocketsBounded(closeWebSocketsWait)
	}

	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Running = false
	})
	log.Printf("serverhost: shutdown complete")
}
