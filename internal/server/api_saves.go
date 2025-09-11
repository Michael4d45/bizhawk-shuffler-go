package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// handleSaveUpload receives multipart save file upload and writes to ./saves directory
func (s *Server) handleSaveUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(32 << 20) // 32MB limit
	if err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("save")
	if err != nil {
		http.Error(w, "save file missing: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	// Use filename from form field if provided, otherwise use uploaded filename
	filename := r.FormValue("filename")
	if filename == "" {
		filename = header.Filename
	}

	// Extract instance ID from filename (remove .state extension)
	instanceID := filename
	if len(filename) > 6 && filename[len(filename)-6:] == ".state" {
		instanceID = filename[:len(filename)-6]
	}

	// Ensure saves directory exists
	savesDir := "./saves"
	if err := os.MkdirAll(savesDir, 0755); err != nil {
		s.setInstanceFileState(instanceID, types.FileStateNone)
		http.Error(w, "failed to create saves dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create destination file
	dstPath := filepath.Join(savesDir, filepath.Base(filename))
	out, err := os.Create(dstPath)
	if err != nil {
		s.setInstanceFileState(instanceID, types.FileStateNone)
		http.Error(w, "create save file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = out.Close() }()

	// Copy file content
	if _, err := io.Copy(out, file); err != nil {
		s.setInstanceFileState(instanceID, types.FileStateNone)
		http.Error(w, "write save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Set state to ready after successful upload
	s.setInstanceFileState(instanceID, types.FileStateReady)

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
		s.setInstanceFileState(instanceID, types.FileStateNone)
		http.Error(w, "save file not found", http.StatusNotFound)
		return
	}

	// Serve the file
	http.ServeFile(w, r, savePath)
}

// setInstanceFileState updates the file state for a given instance ID
func (s *Server) setInstanceFileState(instanceID string, state types.FileState) {
	s.withLock(func() {
		for i, instance := range s.state.GameSwapInstances {
			if instance.ID == instanceID {
				s.state.GameSwapInstances[i].FileState = state
				s.state.UpdatedAt = time.Now()
				break
			}
		}
	})
	_ = s.saveState()
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
						if instance.FileState == types.FileStateReady || instance.FileState == types.FileStateNone {
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
