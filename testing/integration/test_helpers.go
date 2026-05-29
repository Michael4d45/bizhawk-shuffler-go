package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/michael4d45/bizshuffle/savestate"
)

func seedSyncGames(t *testing.T, base string, games []string) {
	t.Helper()
	main := make([]map[string]string, len(games))
	for i, g := range games {
		main[i] = map[string]string{"file": g}
	}
	res, err := postJSON(base, "/api/games", map[string]any{
		"games":      games,
		"main_games": main,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("POST /api/games status %d: %s", res.StatusCode, b)
	}
}

func postAddPlayer(t *testing.T, base, name string) {
	t.Helper()
	res, err := postJSON(base, "/api/add_player", map[string]string{"player": name})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("POST /api/add_player status %d: %s", res.StatusCode, b)
	}
}

func playerAssignment(t *testing.T, base, name string) (game, instanceID string) {
	t.Helper()
	res, err := http.Get(base + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out struct {
		State struct {
			Players map[string]struct {
				Game       string `json:"game"`
				InstanceID string `json:"instance_id"`
			} `json:"players"`
		} `json:"state"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	p, ok := out.State.Players[name]
	if !ok {
		t.Fatalf("player %q missing from state", name)
	}
	return p.Game, p.InstanceID
}

func writeTestROM(t *testing.T, dataDir, filename string) {
	t.Helper()
	dir := filepath.Join(dataDir, "roms")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func postJSON(base, path string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return http.Post(base+path, "application/json", bytes.NewReader(data))
}

func postGameInstances(t *testing.T, base string, instances []map[string]any) {
	t.Helper()
	res, err := postJSON(base, "/api/games", map[string]any{"game_instances": instances})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("POST /api/games status %d: %s", res.StatusCode, b)
	}
}

// UploadMinimalSave posts a valid BizHawk savestate for instanceID to /save/upload.
func UploadMinimalSave(base, instanceID string) error {
	saveBytes, err := savestate.BuildMinimalBizHawkSavestate()
	if err != nil {
		return err
	}
	filename := instanceID + ".state"
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("save", filename)
	if err != nil {
		return err
	}
	if _, err := fw.Write(saveBytes); err != nil {
		return err
	}
	_ = w.WriteField("filename", filename)
	if err := w.Close(); err != nil {
		return err
	}
	res, err := http.Post(base+"/save/upload", w.FormDataContentType(), &buf)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("upload %s: %s", res.Status, b)
	}
	return nil
}

func uploadSaveMultipart(t *testing.T, base, filename string, data []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("save", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	_ = w.WriteField("filename", filename)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(base+"/save/upload", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
