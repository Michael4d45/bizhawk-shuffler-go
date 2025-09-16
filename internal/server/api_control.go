package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// apiStart toggles running=true and notifies clients
func (s *Server) apiStart(w http.ResponseWriter, r *http.Request) {
	s.UpdateStateAndPersist(func(st *types.ServerState) {
		st.Running = true
	})
	s.broadcastToPlayers(types.Command{Cmd: types.CmdResume, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
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

func (s *Server) apiClearSaves(w http.ResponseWriter, r *http.Request) {
	savesDir := "./saves"
	if _, err := os.Stat(savesDir); err == nil {
		trash := fmt.Sprintf("%s.trash.%d", savesDir, time.Now().Unix())
		// Retry rename up to 3 times with small delay to handle Windows file locking issues
		var renameErr error
		for i := range 3 {
			if renameErr = os.Rename(savesDir, trash); renameErr == nil {
				break
			}
			if i < 2 {
				time.Sleep(10 * time.Millisecond)
			}
		}
		if renameErr != nil {
			fmt.Printf("failed to rename saves dir to trash: %v\n", renameErr)
		}
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

func (s *Server) apiTogglePreventSameGame(w http.ResponseWriter, r *http.Request) {
	s.UpdateStateAndPersist(func(st *types.ServerState) {
		st.PreventSameGameSwap = !st.PreventSameGameSwap
	})
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
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// auto fill game catalog
	files, errr := s.getFilesList()
	if errr != nil {
		http.Error(w, "failed to list files: "+errr.Error(), http.StatusInternalServerError)
		return
	}
	s.UpdateStateAndPersist(func(st *types.ServerState) {
		games := st.MainGames
		for _, f := range files {
			// if game not in catalog or is an extra file, add it
			if !slices.ContainsFunc(games, func(g types.GameEntry) bool {
				return g.File == f || slices.Contains(g.ExtraFiles, f)
			}) {
				fmt.Println("Adding game to catalog:", f)
				games = append(games, types.GameEntry{File: f})
			}
		}
		st.MainGames = games
	})

	handler := s.GetGameModeHandler()
	if err := handler.SetupState(); err != nil {
		http.Error(w, "something went wrong "+err.Error(), http.StatusBadRequest)
		return
	}
	s.broadcastGamesUpdate(nil)

	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
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
