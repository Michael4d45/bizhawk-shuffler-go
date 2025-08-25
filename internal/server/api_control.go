package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// apiStart toggles running=true and notifies clients
func (s *Server) apiStart(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.state.Running = true
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	if err := s.saveState(); err != nil {
		// non-fatal; log for visibility
		fmt.Printf("saveState error: %v\n", err)
	}
	s.broadcast(types.Command{Cmd: types.CmdStart, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	select {
	case s.schedulerCh <- struct{}{}:
	default:
	}
	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}

// apiPause toggles running=false and notifies clients
func (s *Server) apiPause(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.state.Running = false
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	if err := s.saveState(); err != nil {
		fmt.Printf("saveState error: %v\n", err)
	}
	s.broadcast(types.Command{Cmd: types.CmdPause, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	select {
	case s.schedulerCh <- struct{}{}:
	default:
	}
	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}

func (s *Server) apiReset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.state.Games = []string{}
	s.state.Running = false
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	if err := s.saveState(); err != nil {
		fmt.Printf("saveState error: %v\n", err)
	}
	s.broadcast(types.Command{Cmd: types.CmdReset, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}

func (s *Server) apiClearSaves(w http.ResponseWriter, r *http.Request) {
	savesDir := "./saves"
	if _, err := os.Stat(savesDir); err == nil {
		trash := fmt.Sprintf("%s.trash.%d", savesDir, time.Now().Unix())
		_ = os.Rename(savesDir, trash)
	}
	_ = os.MkdirAll(savesDir, 0755)
	indexPath := filepath.Join(savesDir, "index.json")
	_ = os.WriteFile(indexPath, []byte("[]"), 0644)
	s.broadcast(types.Command{Cmd: types.CmdClearSaves, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}

func (s *Server) apiToggleSwaps(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.state.SwapEnabled = !s.state.SwapEnabled
	if !s.state.SwapEnabled {
		s.state.NextSwapAt = 0
	}
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	if err := s.saveState(); err != nil {
		fmt.Printf("saveState error: %v\n", err)
	}
	s.broadcast(types.Command{Cmd: types.CmdToggleSwaps, Payload: map[string]any{"enabled": s.state.SwapEnabled, "next_swap_at": s.state.NextSwapAt}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	select {
	case s.schedulerCh <- struct{}{}:
	default:
	}
	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}

// apiMode sets or reads the swap mode
func (s *Server) apiMode(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.Lock()
		mode := s.state.Mode
		s.mu.Unlock()
		if err := json.NewEncoder(w).Encode(map[string]any{"mode": mode}); err != nil {
			fmt.Printf("encode response error: %v\n", err)
		}
		return
	}
	if r.Method == http.MethodPost {
		var b struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.state.Mode = b.Mode
		s.state.UpdatedAt = time.Now()
		s.mu.Unlock()
		if err := s.saveState(); err != nil {
			fmt.Printf("saveState error: %v\n", err)
		}
		if _, err := w.Write([]byte("ok")); err != nil {
			fmt.Printf("write response error: %v\n", err)
		}
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// apiDoSwap triggers an immediate swap
func (s *Server) apiDoSwap(w http.ResponseWriter, r *http.Request) {
	go func() {
		_, _ = s.performSwap()
	}()
	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}
