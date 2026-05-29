package serverhost

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
	"github.com/michael4d45/bizshuffle/savestate"
)

const saveUploadMaxBytes = 32 << 20

// handleSaveUpload receives multipart save file upload and writes to ./saves directory
func (s *Server) handleSaveUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(saveUploadMaxBytes); err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("save")
	if err != nil {
		http.Error(w, "save file missing: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	filename := r.FormValue("filename")
	if filename == "" {
		filename = header.Filename
	}
	filename = filepath.Base(filename)

	instanceID := filename
	if len(filename) > 6 && filename[len(filename)-6:] == ".state" {
		instanceID = filename[:len(filename)-6]
	}

	data, err := io.ReadAll(io.LimitReader(file, saveUploadMaxBytes+1))
	if err != nil {
		http.Error(w, "read save: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(data) > saveUploadMaxBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}

	verified := savestate.VerifyBizHawkSavestate(data, savestate.VerifyOptions{MaxFileBytes: saveUploadMaxBytes})
	if !verified.OK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":   "INVALID_SAVESTATE",
			"code":    verified.Code,
			"message": verified.Message,
			"detail":  verified.Detail,
		})
		return
	}

	savesDir := "./saves"
	if err := os.MkdirAll(savesDir, 0755); err != nil {
		s.setInstanceFileState(instanceID, protocol.FileStateNone)
		http.Error(w, "failed to create saves dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	dstPath := filepath.Join(savesDir, filename)
	if err := os.WriteFile(dstPath, data, 0o644); err != nil {
		s.setInstanceFileState(instanceID, protocol.FileStateNone)
		http.Error(w, "write save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Set state to ready after successful upload
	fmt.Println("Uploaded save file for instance", instanceID, "to", dstPath)
	s.setInstanceFileState(instanceID, protocol.FileStateReady)

	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}

// handleSaveDownload serves save files from ./saves directory
func (s *Server) handleSaveDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract filename from path (everything after /save/)
	path := r.URL.Path
	if len(path) < 6 || path[:6] != "/save/" {
		http.Error(w, "invalid save path", http.StatusBadRequest)
		return
	}
	filename := path[6:] // Remove "/save/" prefix

	if filename == "" {
		http.Error(w, "missing filename", http.StatusBadRequest)
		return
	}

	// Sanitize filename to prevent directory traversal
	filename = filepath.Base(filename)

	// Extract instance ID from filename (remove .state extension)
	instanceID := filename
	if len(filename) > 6 && filename[len(filename)-6:] == ".state" {
		instanceID = filename[:len(filename)-6]
	}

	// Wait for file to be ready (handle pending state)
	if err := s.waitForFileReady(instanceID); err != nil {
		http.Error(w, err.Error(), http.StatusRequestTimeout)
		return
	}

	savePath := filepath.Join("./saves", filename)

	// Check if file exists
	if _, err := os.Stat(savePath); os.IsNotExist(err) {
		// Set state to none if file doesn't exist
		s.setInstanceFileState(instanceID, protocol.FileStateNone)
		http.Error(w, "save file not found", http.StatusNotFound)
		return
	}
	s.setInstanceFileState(instanceID, protocol.FileStateReady)

	// Serve the file
	http.ServeFile(w, r, savePath)
}

// handleNoSaveState handles POST /save/no-save to indicate no save file exists for an instance
func (s *Server) handleNoSaveState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(32 << 20) // 32MB max memory
	if err != nil {
		http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	instanceID := r.FormValue("instance_id")
	if instanceID == "" {
		fmt.Println("instance_id required")
		http.Error(w, "instance_id required", http.StatusBadRequest)
		return
	}
	fmt.Println("handleNoSaveState called for instanceID:", instanceID)

	// Set state to none since there's no save file
	s.setInstanceFileState(instanceID, protocol.FileStateNone)

	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}

// setInstanceFileState updates the file state for a given instance ID
func (s *Server) setInstanceFileState(instanceID string, state protocol.FileState) {
	s.setInstanceFileStateWithPlayer(instanceID, state, "")
}

// setInstanceFileStateWithPlayer updates the file state for a given instance ID and sets the pending player
func (s *Server) setInstanceFileStateWithPlayer(instanceID string, state protocol.FileState, pendingPlayer string) {
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		for i, instance := range st.GameSwapInstances {
			if instance.ID == instanceID {
				if st.GameSwapInstances[i].FileState == state && st.GameSwapInstances[i].PendingPlayer == pendingPlayer {
					// No change
					return
				}
				if state == protocol.FileStatePending {
					s.pendingInstancecount++
				}
				if st.GameSwapInstances[i].FileState == protocol.FileStatePending && state != protocol.FileStatePending {
					s.pendingInstancecount--
				}
				fmt.Println("Setting file state for instance", instanceID, "to", state, "pending player:", pendingPlayer)
				st.GameSwapInstances[i].FileState = state
				st.GameSwapInstances[i].PendingPlayer = pendingPlayer
				break
			}
		}
	})
}

func (s *Server) RequestPendingSaves() {
	// Collect pending saves outside the lock to avoid holding read lock during network operations
	var pendingSaves []struct {
		player     string
		instanceID string
	}
	s.withRLock(func() {
		for _, instance := range s.state.GameSwapInstances {
			if instance.FileState == protocol.FileStatePending && instance.PendingPlayer != "" {
				pendingSaves = append(pendingSaves, struct {
					player     string
					instanceID string
				}{instance.PendingPlayer, instance.ID})
			}
		}
	})

	for _, save := range pendingSaves {
		p := s.currentPlayer(save.player)
		if !s.PlayerReadyForSwap(p) {
			s.clearPendingInstance(save.instanceID)
			continue
		}
		fmt.Println("Requesting save from player", save.player, "for instance", save.instanceID)
		if err := s.RequestSave(save.player, save.instanceID); err != nil {
			fmt.Printf("Failed to request save from player %s for instance %s: %v\n", save.player, save.instanceID, err)
			s.clearPendingInstance(save.instanceID)
		}
	}
}

// waitForFileReady waits for the file state to become ready or none, with timeout
func (s *Server) waitForFileReady(instanceID string) error {
	timeout := time.After(30 * time.Second) // 30-second timeout
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for file %s to be ready", instanceID)
		case <-ticker.C:
			ready := false
			s.withRLock(func() {
				for _, instance := range s.state.GameSwapInstances {
					if instance.ID == instanceID {
						if instance.FileState == protocol.FileStateReady || instance.FileState == protocol.FileStateNone {
							ready = true
						}
						break
					}
				}
			})
			if ready {
				return nil
			}
		}
	}
}
