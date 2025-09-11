package server

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// handleAdmin serves the admin UI index page.
func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./web/index.html")
}

// handleFiles serves files under ./files
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/files/", http.FileServer(http.Dir("./files"))).ServeHTTP(w, r)
}

// handlePluginFiles serves plugin files under ./plugins
func (s *Server) handlePluginFiles(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/files/plugins/", http.FileServer(http.Dir("./plugins"))).ServeHTTP(w, r)
}

// handleUpload receives multipart file upload and writes to ./files directory
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
	dstDir := "./files"
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		http.Error(w, "failed to create files dir: "+err.Error(), http.StatusInternalServerError)
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

// handleFilesList returns a JSON list of files under ./files
func (s *Server) handleFilesList(w http.ResponseWriter, r *http.Request) {
	type fileInfo struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	files := []fileInfo{}
	if err := filepath.Walk("./files", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel("./files", p)
		files = append(files, fileInfo{Name: rel, Size: info.Size()})
		return nil
	}); err != nil {
		http.Error(w, "failed to walk files: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(files); err != nil {
		http.Error(w, "failed to encode files list: "+err.Error(), http.StatusInternalServerError)
		return
	}
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
					if err := os.Rename(tmpName, zipPath); err != nil {
						log.Printf("failed to rename temp zip into place: %v", err)
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
