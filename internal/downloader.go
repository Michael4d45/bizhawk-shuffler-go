package internal

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Downloader fetches files from server files endpoint and stores them under romsDir
type Downloader struct {
	ServerURL string // base server URL, e.g. http://host:port
	RomsDir   string
	Client    *http.Client
}

// NewDownloader creates a downloader; if romsDir is empty uses ./roms
func NewDownloader(serverURL, romsDir string) *Downloader {
	if romsDir == "" {
		romsDir = "./roms"
	}
	return &Downloader{ServerURL: serverURL, RomsDir: romsDir, Client: &http.Client{Timeout: 30 * time.Second}}
}

// EnsureFile ensures the named file exists locally, downloading it from the server if missing.
// name is the relative path under /files/ on the server (e.g. "games/foo.zip").
func (d *Downloader) EnsureFile(ctx context.Context, name string) error {
	dest := filepath.Join(d.RomsDir, filepath.FromSlash(name))
	if _, err := os.Stat(dest); err == nil {
		return nil // exists
	}
	// ensure directory
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	// build URL
	url := d.ServerURL
	if len(url) > 0 && url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}
	fetch := url + "/files/" + name

	// try up to 3 times
	var lastErr error
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", fetch, nil)
		resp, err := d.Client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("bad status: %s", resp.Status)
			resp.Body.Close()
			time.Sleep(500 * time.Millisecond)
			continue
		}
		out, err := os.Create(dest)
		if err != nil {
			resp.Body.Close()
			return err
		}
		_, err = io.Copy(out, resp.Body)
		resp.Body.Close()
		out.Close()
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return nil
	}
	return lastErr
}
