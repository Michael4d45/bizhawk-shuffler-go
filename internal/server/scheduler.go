package server

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// performSwap dispatches to the appropriate mode implementation.
func (s *Server) performSwap() error {
	s.mu.Lock()
	mode := s.state.Mode
	s.mu.Unlock()

	handler, err := getGameModeHandler(mode)
	if err != nil {
		return err
	}

	return handler.HandleSwap(s)
}

// schedulerLoop schedules automatic swaps when enabled.
func (s *Server) schedulerLoop() {
	for {
		s.mu.Lock()
		running := s.state.Running
		enabled := s.state.SwapEnabled
		s.mu.Unlock()
		if !running || !enabled {
			<-s.schedulerCh
			continue
		}
		s.mu.Lock()
		minv := s.state.MinIntervalSecs
		maxv := s.state.MaxIntervalSecs
		s.mu.Unlock()
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
		s.mu.Lock()
		s.state.NextSwapAt = nextAt
		s.state.UpdatedAt = time.Now()
		s.mu.Unlock()
		if err := s.saveState(); err != nil {
			fmt.Printf("saveState error: %v\n", err)
		}
		s.broadcast(types.Command{Cmd: types.CmdStateUpdate, Payload: map[string]any{"next_swap_at": nextAt, "updated_at": time.Now()}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
		timer := time.NewTimer(time.Duration(interval) * time.Second)
		select {
		case <-timer.C:
		case <-s.schedulerCh:
			if !timer.Stop() {
				<-timer.C
			}
			continue
		}
		s.mu.Lock()
		if !s.state.Running || !s.state.SwapEnabled {
			s.mu.Unlock()
			continue
		}
		s.mu.Unlock()
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
