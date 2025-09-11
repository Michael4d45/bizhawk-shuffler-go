package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// apiStart toggles running=true and notifies clients
func (s *Server) apiStart(w http.ResponseWriter, r *http.Request) {
	s.UpdateStateAndPersist(func(st *types.ServerState) {
		st.Running = true
	})
	s.broadcastToPlayers(types.Command{Cmd: types.CmdStart, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
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
	s.UpdateStateAndPersist(func(st *types.ServerState) {
		st.Running = false
	})
	s.broadcastToPlayers(types.Command{Cmd: types.CmdPause, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	select {
	case s.schedulerCh <- struct{}{}:
	default:
	}
	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}

func (s *Server) apiReset(w http.ResponseWriter, r *http.Request) {
	s.UpdateStateAndPersist(func(st *types.ServerState) {
		st.GameSwapInstances = []types.GameSwapInstance{}
		st.Running = false
	})
	s.broadcastToPlayers(types.Command{Cmd: types.CmdReset, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
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
	s.broadcastToPlayers(types.Command{Cmd: types.CmdClearSaves, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}

func (s *Server) apiToggleSwaps(w http.ResponseWriter, r *http.Request) {
	s.UpdateStateAndPersist(func(st *types.ServerState) {
		st.SwapEnabled = !st.SwapEnabled
		if !st.SwapEnabled {
			st.NextSwapAt = 0
		}
	})
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
		var mode types.GameMode
		s.withRLock(func() { mode = s.state.Mode })
		if err := json.NewEncoder(w).Encode(map[string]any{"mode": mode}); err != nil {
			fmt.Printf("encode response error: %v\n", err)
		}
		return
	}
	if r.Method == http.MethodPost {
		var b struct {
			Mode types.GameMode `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		s.UpdateStateAndPersist(func(st *types.ServerState) {
			st.Mode = b.Mode
		})
		if _, err := w.Write([]byte("ok")); err != nil {
			fmt.Printf("write response error: %v\n", err)
		}
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// apiMode sets or reads the swap mode
func (s *Server) apiModeSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {

		handler := s.GetGameModeHandler()
		if err := handler.SetupState(); err != nil {
			http.Error(w, "something went wrong "+err.Error(), http.StatusBadRequest)
			return
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
		if err := s.performSwap(); err != nil {
			fmt.Printf("performSwap error: %v\n", err)
		}
	}()
	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}
