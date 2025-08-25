package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// handleSaveUpload accepts multipart form upload with field `save` and optional form fields `player` and `game`.
func (s *Server) handleSaveUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	player := r.FormValue("player")
	game := r.FormValue("game")
	file, header, err := r.FormFile("save")
	if err != nil {
		http.Error(w, "save file missing: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()
	if player == "" {
		player = "unknown"
	}
	dir := filepath.Join("./saves", filepath.Base(player))
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, "mkdir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var fname string
	if f := r.FormValue("filename"); f != "" {
		fname = f
	} else if header.Filename != "" {
		fname = header.Filename
	} else if game != "" {
		fname = game + ".state"
	} else {
		fname = fmt.Sprintf("save-%d.state", time.Now().UnixNano())
	}
	fname = filepath.Base(fname)
	tmp := filepath.Join(dir, fname+".tmp")
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		http.Error(w, "create tmp: "+err.Error(), http.StatusInternalServerError)
		return
	}
	lr := &io.LimitedReader{R: file, N: maxSaveSize}
	if _, err := io.Copy(out, lr); err != nil {
		out.Close()
		os.Remove(tmp)
		http.Error(w, "write tmp: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if lr.N == 0 {
		out.Close()
		os.Remove(tmp)
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	out.Close()
	dst := filepath.Join(dir, fname)
	if err := os.Rename(tmp, dst); err != nil {
		log.Printf("failed to rename tmp index: %v", err)
		os.Remove(tmp)
		http.Error(w, "rename: "+err.Error(), http.StatusInternalServerError)
		return
	}

	indexPath := filepath.Join("./saves", "index.json")
	var idx []SaveIndexEntry
	s.mu.Lock()
	if b, err := os.ReadFile(indexPath); err == nil {
		_ = json.Unmarshal(b, &idx)
	}
	newIdx := make([]SaveIndexEntry, 0, len(idx)+1)
	for _, e := range idx {
		if !(e.Player == player && e.File == fname) {
			newIdx = append(newIdx, e)
		}
	}
	fi, _ := os.Stat(dst)
	newIdx = append(newIdx, SaveIndexEntry{Player: player, File: fname, Size: fi.Size(), At: time.Now().Unix(), Game: game})
	tmpIndex := indexPath + ".tmp"
	ib, _ := json.MarshalIndent(newIdx, "", "  ")
	if err := os.WriteFile(tmpIndex, ib, 0644); err == nil {
		os.Rename(tmpIndex, indexPath)
	}
	s.mu.Unlock()

	go s.reindexSaves()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"result": "ok", "file": dst}); err != nil {
		http.Error(w, "failed to encode response: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleSaveServe serves files under ./saves via /save/<player>/<file>
func (s *Server) handleSaveServe(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/save/")
	if rel == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	rel = strings.TrimPrefix(rel, "/")
	rel = path.Clean(rel)
	parts := strings.SplitN(rel, "/", 2)
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	p := filepath.Join("./saves", parts[0], parts[1])
	http.ServeFile(w, r, p)
}

// handleSavesList returns a JSON listing of saves under ./saves/<player> directories
func (s *Server) handleSavesList(w http.ResponseWriter, r *http.Request) {
	type SaveInfo struct {
		Player string `json:"player"`
		File   string `json:"file"`
		Size   int64  `json:"size"`
	}
	indexPath := filepath.Join("./saves", "index.json")
	if b, err := os.ReadFile(indexPath); err == nil {
		var idx []SaveInfo
		if err := json.Unmarshal(b, &idx); err == nil {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(idx); err == nil {
				return
			}
		}
	}
	saves := []SaveInfo{}
	if err := filepath.Walk("./saves", func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel("./saves", p)
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		if len(parts) == 2 {
			saves = append(saves, SaveInfo{Player: parts[0], File: parts[1], Size: info.Size()})
		}
		return nil
	}); err != nil {
		log.Printf("walk saves error: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(saves); err != nil {
		http.Error(w, "failed to encode saves: "+err.Error(), http.StatusInternalServerError)
		return
	}
}
