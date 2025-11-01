package server

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// handleAdmin serves the admin UI index page.
func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./web/index.html")
}

// handleFiles serves files under ./roms
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/files/", http.FileServer(http.Dir("./roms"))).ServeHTTP(w, r)
}

// handlePluginFiles serves plugin files under ./plugins
func (s *Server) handlePluginFiles(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/files/plugins/", http.FileServer(http.Dir("./plugins"))).ServeHTTP(w, r)
}

// handleUpload receives multipart file upload and writes to ./roms directory
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file missing: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()
	dstDir := "./roms"
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		http.Error(w, "failed to create roms dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	dstPath := filepath.Join(dstDir, filepath.Base(header.Filename))
	out, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, "create file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, "write file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := w.Write([]byte("ok")); err != nil {
		fmt.Printf("write response error: %v\n", err)
	}
}

// handleFilesList returns a JSON list of files under ./roms
func (s *Server) handleFilesList(w http.ResponseWriter, r *http.Request) {
	files, err := s.getFilesList()
	if err != nil {
		http.Error(w, "failed to list files: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(files); err != nil {
		http.Error(w, "failed to encode files list: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) getFilesList() ([]string, error) {
	files := []string{}
	if err := filepath.Walk("./roms", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel("./roms", p)
		files = append(files, rel)
		return nil
	}); err != nil {
		return nil, err
	}
	return files, nil
}

// handleBizhawkFilesZip serves a BizhawkFiles.zip by streaming or creating a zip
func (s *Server) handleBizhawkFilesZip(w http.ResponseWriter, r *http.Request) {
	zipPath := filepath.Join("./web", "BizhawkFiles.zip")
	if fi, err := os.Stat(zipPath); err == nil && !fi.IsDir() {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=BizhawkFiles.zip")
		http.ServeFile(w, r, zipPath)
		return
	}
	dir := filepath.Join("./web", "BizhawkFiles")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		http.Error(w, "BizhawkFiles not found", http.StatusNotFound)
		return
	}

	if fi, err := os.Stat(zipPath); err != nil || fi.IsDir() {
		if err := os.MkdirAll(filepath.Dir(zipPath), 0755); err != nil {
			log.Printf("failed to ensure web dir for zip: %v", err)
		} else {
			tmp, err := os.CreateTemp(filepath.Dir(zipPath), "BizhawkFiles-*.zip.tmp")
			if err != nil {
				log.Printf("failed to create temp zip file: %v", err)
			} else {
				tmpName := tmp.Name()
				if err := tmp.Close(); err != nil {
					log.Printf("tmp close error: %v", err)
				}
				if err := func() error {
					f, err := os.OpenFile(tmpName, os.O_WRONLY|os.O_TRUNC, 0644)
					if err != nil {
						return err
					}
					defer func() { _ = f.Close() }()
					if err := zipDir(dir, f); err != nil {
						return err
					}
					_ = f.Sync()
					return nil
				}(); err != nil {
					log.Printf("failed to build BizhawkFiles.zip to temp: %v", err)
					_ = os.Remove(tmpName)
				} else {
					// Retry rename up to 3 times with small delay to handle Windows file locking issues
					var renameErr error
					for i := 0; i < 3; i++ {
						if renameErr = os.Rename(tmpName, zipPath); renameErr == nil {
							break
						}
						if i < 2 {
							time.Sleep(10 * time.Millisecond)
						}
					}
					if renameErr != nil {
						log.Printf("failed to rename temp zip into place: %v", renameErr)
						_ = os.Remove(tmpName)
					}
				}
			}
		}
		if fi, err := os.Stat(zipPath); err == nil && !fi.IsDir() {
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", "attachment; filename=BizhawkFiles.zip")
			http.ServeFile(w, r, zipPath)
			return
		}
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=BizhawkFiles.zip")
	if err := zipDir(dir, w); err != nil {
		log.Printf("failed to stream BizhawkFiles.zip: %v", err)
		http.Error(w, "failed to create zip", http.StatusInternalServerError)
		return
	}
}

// zipDir writes a zip archive of srcDir to the provided writer.
func zipDir(srcDir string, w io.Writer) error {
	zw := zip.NewWriter(w)
	defer func() { _ = zw.Close() }()
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		name := filepath.ToSlash(rel)
		if info.IsDir() {
			_, err := zw.Create(name + "/")
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		fh, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		fh.Name = name
		fh.Method = zip.Deflate
		wtr, err := zw.CreateHeader(fh)
		if err != nil {
			return err
		}
		_, err = io.Copy(wtr, f)
		return err
	})
}

// handleOpenRomsFolder opens the roms folder in the system file manager
func (s *Server) handleOpenRomsFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	romsDir, err := filepath.Abs("./roms")
	if err != nil {
		http.Error(w, "failed to resolve roms directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure directory exists
	if err := os.MkdirAll(romsDir, 0755); err != nil {
		http.Error(w, "failed to create roms directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var err2 error
	switch runtime.GOOS {
	case "windows":
		err2 = exec.Command("explorer", romsDir).Start()
	case "darwin":
		err2 = exec.Command("open", romsDir).Start()
	case "linux":
		err2 = exec.Command("xdg-open", romsDir).Start()
	default:
		http.Error(w, "unsupported platform: "+runtime.GOOS, http.StatusNotImplemented)
		return
	}

	if err2 != nil {
		log.Printf("Failed to open roms folder: %v", err2)
		http.Error(w, "failed to open folder: "+err2.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("encode response error: %v", err)
	}
}

// handleOpenPluginsFolder opens the plugins folder in the system file manager
func (s *Server) handleOpenPluginsFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pluginsDir, err := filepath.Abs("./plugins")
	if err != nil {
		http.Error(w, "failed to resolve plugins directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure directory exists
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		http.Error(w, "failed to create plugins directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var err2 error
	switch runtime.GOOS {
	case "windows":
		err2 = exec.Command("explorer", pluginsDir).Start()
	case "darwin":
		err2 = exec.Command("open", pluginsDir).Start()
	case "linux":
		err2 = exec.Command("xdg-open", pluginsDir).Start()
	default:
		http.Error(w, "unsupported platform: "+runtime.GOOS, http.StatusNotImplemented)
		return
	}

	if err2 != nil {
		log.Printf("Failed to open plugins folder: %v", err2)
		http.Error(w, "failed to open folder: "+err2.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("encode response error: %v", err)
	}
}
