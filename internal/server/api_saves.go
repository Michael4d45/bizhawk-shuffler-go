package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

	// Ensure saves directory exists
	savesDir := "./saves"
	if err := os.MkdirAll(savesDir, 0755); err != nil {
		http.Error(w, "failed to create saves dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create destination file
	dstPath := filepath.Join(savesDir, filepath.Base(filename))
	out, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, "create save file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = out.Close() }()

	// Copy file content
	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, "write save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

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

	savePath := filepath.Join("./saves", filename)

	// Check if file exists
	if _, err := os.Stat(savePath); os.IsNotExist(err) {
		http.Error(w, "save file not found", http.StatusNotFound)
		return
	}

	// Serve the file
	http.ServeFile(w, r, savePath)
}
