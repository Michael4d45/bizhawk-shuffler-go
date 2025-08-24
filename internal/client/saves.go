package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// UploadSave uploads a local save file to the server. It does not emit status
// events; callers should handle UI/status notifications.
func UploadSave(serverHTTP, localPath, player, game string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("save", filepath.Base(localPath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return err
	}
	_ = w.WriteField("player", player)
	_ = w.WriteField("game", game)
	_ = w.WriteField("filename", filepath.Base(localPath))
	w.Close()
	req, err := http.NewRequestWithContext(context.Background(), "POST", strings.TrimRight(serverHTTP, "/")+"/save/upload", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: %s %s", resp.Status, string(data))
	}
	return nil
}

// DownloadSave downloads a save file for player/filename into ./saves/player.
// Returns ErrNotFound when the server responds 404.
func DownloadSave(ctx context.Context, serverHTTP, player, filename string) error {
	destDir := filepath.Join("./saves", player)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	base := strings.TrimSuffix(serverHTTP, "/")
	p := "/save/" + url.PathEscape(player) + "/" + url.PathEscape(filename)
	fetch := base + p
	req, _ := http.NewRequestWithContext(ctx, "GET", fetch, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == http.StatusNotFound {
			return ErrNotFound
		}
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bad status: %s %s", resp.Status, string(body))
	}
	outPath := filepath.Join(destDir, filename)
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
