package serverhost

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strings"
)

//go:embed static/*
var embeddedStatic embed.FS

func adminHTTPFS() http.FileSystem {
	if dir := os.Getenv("BIZSHUFFLE_STATIC_DIR"); dir != "" {
		return http.Dir(dir)
	}
	sub, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		return http.Dir("static")
	}
	return http.FS(sub)
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if strings.Contains(path, "..") {
		http.NotFound(w, r)
		return
	}
	http.FileServer(adminHTTPFS()).ServeHTTP(w, r)
}
