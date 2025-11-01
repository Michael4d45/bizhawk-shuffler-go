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

func (s *Server) sendMessage(message string, duration int, x int, y int, fontsize int, fg string, bg string) {
	s.broadcastToPlayers(types.Command{
		Cmd: types.CmdMessage,
		Payload: map[string]any{
			"message":  message,
			"duration": duration,
			"x":        x,
			"y":        y,
			"fontsize": fontsize,
			"fg":       fg,
			"bg":       bg,
		},
		ID: fmt.Sprintf("message-%s-%d", message, time.Now().UnixNano()),
	})
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
		var countdownEnabled bool
		s.mu.RLock()
		countdownEnabled = s.state.CountdownEnabled
		s.mu.RUnlock()

		// Send countdown messages if enabled and interval is long enough
		if countdownEnabled && interval >= 3 {
			// Wait until 3 seconds before swap
			countdownDelay := interval - 3
			if countdownDelay > 0 {
				countdownTimer := time.NewTimer(time.Duration(countdownDelay) * time.Second)
				select {
				case <-countdownTimer.C:
				case <-s.schedulerCh:
					if !countdownTimer.Stop() {
						<-countdownTimer.C
					}
					continue
				}
			}

			// Check if still running before countdown
			s.mu.RLock()
			if !s.state.Running || !s.state.SwapEnabled {
				s.mu.RUnlock()
				continue
			}
			s.mu.RUnlock()

			// Send "3" message
			s.sendMessage("3", 1, 10, 10, 12, "#FFFFFF", "#000000")

			// Wait 1 second for "2"
			countdownTimer := time.NewTimer(1 * time.Second)
			select {
			case <-countdownTimer.C:
				s.sendMessage("2", 1, 10, 10, 12, "#FFFFFF", "#000000")
			case <-s.schedulerCh:
				if !countdownTimer.Stop() {
					<-countdownTimer.C
				}
				continue
			}

			// Wait 1 second for "1"
			countdownTimer = time.NewTimer(1 * time.Second)
			select {
			case <-countdownTimer.C:
				s.sendMessage("1", 1, 10, 10, 12, "#FFFFFF", "#000000")
			case <-s.schedulerCh:
				if !countdownTimer.Stop() {
					<-countdownTimer.C
				}
				continue
			}

			// Check again if still running before performing swap
			s.mu.RLock()
			if !s.state.Running || !s.state.SwapEnabled {
				s.mu.RUnlock()
				continue
			}
			s.mu.RUnlock()
		} else {
			// No countdown: wait for full interval
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
		}

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
