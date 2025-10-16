package server

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// performSwap dispatches to the appropriate mode implementation.
func (s *Server) performSwap() error {
	handler := s.GetGameModeHandler()
	// Call the mode-specific swap handler.
	if err := handler.HandleSwap(); err != nil {
		return err
	}
	return nil
}

func (s *Server) performRandomSwapForPlayer(playerName string) any {
	handler := s.GetGameModeHandler()
	// Call the mode-specific swap handler.
	if err := handler.HandleRandomSwapForPlayer(playerName); err != nil {
		return err
	}
	return nil
}

// schedulerLoop schedules automatic swaps when enabled.
func (s *Server) schedulerLoop() {
	for {
		s.mu.RLock()
		running := s.state.Running
		enabled := s.state.SwapEnabled
		s.mu.RUnlock()
		if !running || !enabled {
			<-s.schedulerCh
			continue
		}
		s.mu.RLock()
		minv := s.state.MinIntervalSecs
		maxv := s.state.MaxIntervalSecs
		s.mu.RUnlock()
		var interval int
		if minv > 0 && maxv > 0 && maxv >= minv {
			interval = minv + rand.Intn(maxv-minv+1)
		} else if minv > 0 {
			interval = minv
		} else if maxv > 0 {
			interval = maxv
		} else {
			interval = 300
		}
		nextAt := time.Now().Add(time.Duration(interval) * time.Second).Unix()
		s.UpdateStateAndPersist(func(st *types.ServerState) {
			st.NextSwapAt = nextAt
		})
		timer := time.NewTimer(time.Duration(interval) * time.Second)
		select {
		case <-timer.C:
		case <-s.schedulerCh:
			if !timer.Stop() {
				<-timer.C
			}
			continue
		}
		s.mu.RLock()
		if !s.state.Running || !s.state.SwapEnabled {
			s.mu.RUnlock()
			continue
		}
		s.mu.RUnlock()
		go func() {
			err := s.performSwap()
			if err != nil {
				fmt.Printf("performSwap error: %v\n", err)
			}
			select {
			case s.schedulerCh <- struct{}{}:
			default:
			}
		}()
	}
}
