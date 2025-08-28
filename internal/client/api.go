package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// API centralises HTTP interactions with the server for the client.
type API struct {
	BaseURL    string
	HTTPClient *http.Client
	cfg        Config
	Ctx        context.Context
}

// NewAPI constructs an API instance. base may be empty.
func NewAPI(base string, httpClient *http.Client, cfg Config) *API {
	return NewAPIWithContext(base, httpClient, cfg, context.Background())
}

// NewAPIWithContext constructs an API instance that uses the provided context for requests.
func NewAPIWithContext(base string, httpClient *http.Client, cfg Config, ctx context.Context) *API {
	if ctx == nil {
		ctx = context.Background()
	}
	return &API{BaseURL: strings.TrimRight(base, "/"), HTTPClient: httpClient, cfg: cfg, Ctx: ctx}
}

// GetState fetches /state.json and decodes the envelope into the provided dest.
func (a *API) GetState(dest interface{}) error {
	if a.BaseURL == "" {
		return fmt.Errorf("no server configured")
	}
	req, err := http.NewRequestWithContext(a.Ctx, "GET", a.BaseURL+"/state.json", nil)
	if err != nil {
		return err
	}
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bad status %s: %s", resp.Status, string(b))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// FetchServerState fetches the server state and extracts whether the server
// is running and the current game for the given player name (if any).
// It returns (running, playerGame, error).
func (a *API) FetchServerState(player string) (bool, string, string, error) {
	var env struct {
		State struct {
			Running bool                      `json:"running"`
			Players map[string]map[string]any `json:"players"`
		} `json:"state"`
	}
	if err := a.GetState(&env); err != nil {
		return false, "", "", err
	}
	running := env.State.Running
	playerGame := ""
	instanceID := ""
	if env.State.Players != nil {
		if p, ok := env.State.Players[player]; ok {
			if v, ok2 := p["game"]; ok2 {
				if s, ok3 := v.(string); ok3 {
					playerGame = s
				}
			}
			if v, ok2 := p["instance_id"]; ok2 {
				if s, ok3 := v.(string); ok3 {
					instanceID = s
				}
			}
		}
	}
	return running, playerGame, instanceID, nil
}

// UploadSave uploads a local save file to the server.
func (a *API) UploadSaveState(instanceID string) error {
	localPath := "./saves/" + instanceID + ".state"
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("save", filepath.Base(localPath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return err
	}
	_ = w.WriteField("filename", filepath.Base(localPath))
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(a.Ctx, "POST", a.BaseURL+"/save/upload", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: %s %s", resp.Status, string(data))
	}
	return nil
}

// DownloadSave downloads a save file for player/filename into ./saves/player.
// Returns ErrNotFound when the server responds 404.
func (a *API) EnsureSaveState(instanceID string) error {
	p := "/save/" + url.PathEscape(instanceID+".state")
	fetch := a.BaseURL + p
	req, _ := http.NewRequestWithContext(a.Ctx, "GET", fetch, nil)
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		if resp.StatusCode == http.StatusNotFound {
			return ErrNotFound
		}
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bad status: %s %s", resp.Status, string(body))
	}
	outPath := filepath.Join("./saves", instanceID+".state")
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, resp.Body)
	return err
}

// GamesResponse mirrors the server /api/games response.
type GamesResponse struct {
	GameInstances []types.GameSwapInstance `json:"game_instances"`
	MainGames     []types.GameEntry        `json:"main_games"`
}

// GetGames fetches /api/games and returns the parsed response.
func (a *API) GetGames() (GamesResponse, error) {
	var resp GamesResponse
	if a.BaseURL == "" {
		return resp, fmt.Errorf("no server configured")
	}
	r, err := a.HTTPClient.Get(a.BaseURL + "/api/games")
	if err != nil {
		return resp, err
	}
	defer func() { _ = r.Body.Close() }()
	if r.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r.Body)
		return resp, fmt.Errorf("bad status %s: %s", r.Status, string(b))
	}
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// UpdateGames posts new games and main_games to the server.
func (a *API) UpdateGames(instances []types.GameSwapInstance, mainGames []types.GameEntry) error {
	if a.BaseURL == "" {
		return fmt.Errorf("no server configured")
	}
	body := map[string]any{"game_instances": instances, "main_games": mainGames}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(context.Background(), "POST", a.BaseURL+"/api/games", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update games failed: %s %s", resp.Status, string(data))
	}
	return nil
}

// GetInterval retrieves min/max interval seconds from /api/interval.
func (a *API) GetInterval() (min int, max int, err error) {
	if a.BaseURL == "" {
		return 0, 0, fmt.Errorf("no server configured")
	}
	r, err := a.HTTPClient.Get(a.BaseURL + "/api/interval")
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = r.Body.Close() }()
	if r.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r.Body)
		return 0, 0, fmt.Errorf("bad status %s: %s", r.Status, string(b))
	}
	var out struct {
		MinInterval int `json:"min_interval_secs"`
		MaxInterval int `json:"max_interval_secs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		return 0, 0, err
	}
	return out.MinInterval, out.MaxInterval, nil
}

// SetInterval sets min/max interval seconds.
func (a *API) SetInterval(min, max int) error {
	if a.BaseURL == "" {
		return fmt.Errorf("no server configured")
	}
	body := map[string]int{"min_interval_secs": min, "max_interval_secs": max}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(context.Background(), "POST", a.BaseURL+"/api/interval", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("set interval failed: %s %s", resp.Status, string(data))
	}
	return nil
}

// SwapPlayer sends a swap command for a single player and returns the result.
func (a *API) SwapPlayerToInstance(player, instanceID string) (string, error) {
	if a.BaseURL == "" {
		return "", fmt.Errorf("no server configured")
	}
	body := map[string]string{"player": player, "instance_id": instanceID}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(context.Background(), "POST", a.BaseURL+"/api/swap_player", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("swap player failed: %s %s", resp.Status, string(data))
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out["result"], nil
}

// RemovePlayer removes a player via POST /api/remove_player and returns the server result.
func (a *API) RemovePlayer(player string) (string, error) {
	if a.BaseURL == "" {
		return "", fmt.Errorf("no server configured")
	}
	body := map[string]string{"player": player}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(context.Background(), "POST", a.BaseURL+"/api/remove_player", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("remove player failed: %s %s", resp.Status, string(data))
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out["result"], nil
}

// SwapAllToGame requests swaps for all players to the given game and returns the results per player.
func (a *API) SwapAllToGame(game string) (map[string]string, error) {
	if a.BaseURL == "" {
		return nil, fmt.Errorf("no server configured")
	}
	body := map[string]string{"game": game}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(context.Background(), "POST", a.BaseURL+"/api/swap_all_to_game", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("swap all failed: %s %s", resp.Status, string(data))
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// FileInfo mirrors the server file list entry.
type FileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// ListFiles returns the server files list from /files/list.json.
func (a *API) ListFiles() ([]FileInfo, error) {
	if a.BaseURL == "" {
		return nil, fmt.Errorf("no server configured")
	}
	r, err := a.HTTPClient.Get(a.BaseURL + "/files/list.json")
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Body.Close() }()
	if r.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r.Body)
		return nil, fmt.Errorf("bad status %s: %s", r.Status, string(b))
	}
	var files []FileInfo
	if err := json.NewDecoder(r.Body).Decode(&files); err != nil {
		return nil, err
	}
	return files, nil
}

// UploadFile uploads a local file to /upload using form field "file".
func (a *API) UploadFile(localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(context.Background(), "POST", a.BaseURL+"/upload", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: %s %s", resp.Status, string(data))
	}
	return nil
}

// BizhawkFilesURL returns the URL to download BizhawkFiles.zip from the server.
func (a *API) BizhawkFilesURL() string {
	return a.BaseURL + "/api/BizhawkFiles.zip"
}

// EnsureFile ensures the named file exists locally, downloading it from the server if missing.
// name is the relative path under /files/ on the server (e.g. "games/foo.zip").
func (a *API) EnsureFile(ctx context.Context, name string) error {
	dest := filepath.Join("./roms", filepath.FromSlash(name))
	if _, err := os.Stat(dest); err == nil {
		return nil // exists
	}
	// ensure directory
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	// build URL
	fetch := a.BaseURL
	if len(fetch) > 0 && fetch[len(fetch)-1] == '/' {
		fetch = fetch[:len(fetch)-1]
	}
	fetch += "/files/" + name

	// try up to 3 times
	var lastErr error
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", fetch, nil)
		resp, err := a.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("bad status: %s", resp.Status)
			_ = resp.Body.Close()
			time.Sleep(500 * time.Millisecond)
			continue
		}
		out, err := os.Create(dest)
		if err != nil {
			_ = resp.Body.Close()
			return err
		}
		_, err = io.Copy(out, resp.Body)
		_ = resp.Body.Close()
		if err := out.Close(); err != nil {
			_ = err
		}
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return nil
	}
	return lastErr
}
