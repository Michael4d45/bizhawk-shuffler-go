package installer

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// Downloader handles file downloads
type Downloader struct {
	httpClient *http.Client
}

// NewDownloader creates a new downloader
func NewDownloader() *Downloader {
	return &Downloader{
		httpClient: &http.Client{},
	}
}

// DownloadFile downloads a file from a URL to a destination path
func (d *Downloader) DownloadFile(url, dest string, progress func(current, total int64)) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	resp, err := d.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status %s", resp.Status)
	}

	total := resp.ContentLength
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = out.Close() }()

	var current int64
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			nw, ew := out.Write(buf[0:nr])
			if ew != nil {
				return fmt.Errorf("write error: %w", ew)
			}
			if nr != nw {
				return fmt.Errorf("short write")
			}
			current += int64(nw)
			if progress != nil {
				progress(current, total)
			}
		}
		if er != nil {
			if er != io.EOF {
				return fmt.Errorf("read error: %w", er)
			}
			break
		}
	}

	return nil
}

// GetAssetNameForPlatform returns the expected asset name for the current platform
func GetAssetNameForPlatform(component string) string {
	// Assets are named like: bizshuffle-server-windows-amd64.zip or bizshuffle-client-windows-amd64.zip
	return fmt.Sprintf("bizshuffle-%s-%s-%s.zip", component, runtime.GOOS, runtime.GOARCH)
}
