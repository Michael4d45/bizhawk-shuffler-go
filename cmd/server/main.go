package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yourname/bizshuffle/internal/types"
)

var addr string
var ErrTimeout = errors.New("timeout waiting for result")

const maxSaveSize = 50 << 20 // 50 MiB

type Server struct {
	mu    sync.Mutex
	state types.ServerState
	// wsClient represents a connected client and its send queue
	// defined below as a type alias
	conns map[*websocket.Conn]*wsClient
	// players maps player name -> client for quick lookup
	players   map[string]*wsClient
	upgrader  websocket.Upgrader
	stateFile string
	// pending maps a command ID -> channel used to collect a single ack/nack result
	pending map[string]chan string
	// ephemeral status per player (uploading/downloading/idle)
	ephemeral map[string]string
	// schedulerCh is used to wake the scheduler when state changes (start/pause/toggle)
	schedulerCh chan struct{}
}

// SaveIndexEntry is a single entry in ./saves/index.json
type SaveIndexEntry struct {
	Player string `json:"player"`
	File   string `json:"file"`
	Size   int64  `json:"size"`
	At     int64  `json:"at"`
	Game   string `json:"game"`
}

// wsClient represents a connected websocket client and its outbound send queue
type wsClient struct {
	conn   *websocket.Conn
	sendCh chan types.Command
}

func main() {
	host := flag.String("host", "127.0.0.1", "host to bind")
	port := flag.Int("port", 8080, "port to bind")
	flag.Parse()
	addr = fmt.Sprintf("%s:%d", *host, *port)

	root, _ := os.Getwd()
	stateFile := filepath.Join(root, "state.json")

	s := NewServer(stateFile)

	http.HandleFunc("/ws", s.handleWS)
	http.HandleFunc("/", s.handleAdmin)
	// control APIs
	http.HandleFunc("/api/start", s.apiStart)
	http.HandleFunc("/api/pause", s.apiPause)
	http.HandleFunc("/api/reset", s.apiReset)
	http.HandleFunc("/api/clear_saves", s.apiClearSaves)
	http.HandleFunc("/api/toggle_swaps", s.apiToggleSwaps)
	http.HandleFunc("/api/do_swap", s.apiDoSwap)
	http.HandleFunc("/api/mode", s.apiMode)
	http.HandleFunc("/files/", s.handleFiles)
	http.HandleFunc("/upload", s.handleUpload)
	http.HandleFunc("/upload/state", s.handleSaveUpload)
	http.HandleFunc("/files/list.json", s.handleFilesList)
	http.HandleFunc("/api/BizhawkFiles.zip", s.handleBizhawkFilesZip)
	http.HandleFunc("/state.json", s.handleStateJSON)
	http.HandleFunc("/saves/list.json", s.handleSavesList)
	http.HandleFunc("/api/games", s.apiGames)
	http.HandleFunc("/api/interval", s.apiInterval)
	http.HandleFunc("/api/swap_player", s.apiSwapPlayer)
	http.HandleFunc("/api/remove_player", s.apiRemovePlayer)
	http.HandleFunc("/api/swap_all_to_game", s.apiSwapAllToGame)
	http.HandleFunc("/save/upload", s.handleSaveUpload)
	http.HandleFunc("/save/", s.handleSaveServe)
	log.Printf("Starting server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// apiDoSwap triggers a swap round: compute mapping, notify clients to save+upload,
// wait for their ack, then instruct clients to download the incoming saves and load.
func (s *Server) apiDoSwap(w http.ResponseWriter, r *http.Request) {
	outcome, err := s.performSwap()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// If any swap ack failed, report partial_failure for backwards compatibility
	failed := false
	for _, r := range outcome.Results {
		if r != "ack" {
			failed = true
			break
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if failed {
		json.NewEncoder(w).Encode(map[string]any{"status": "partial_failure", "results": outcome.Results, "download_results": outcome.DownloadResults})
	} else {
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "swap_results": outcome.Results, "download_results": outcome.DownloadResults})
	}
}

// apiMode handles GET/POST to view or set the current swap mode
// GET returns {mode: "sync"|"save"}
// POST accepts {mode: "sync"|"save"}
func (s *Server) apiMode(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		mode := s.state.Mode
		s.mu.Unlock()
		if mode == "" {
			mode = "sync"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"mode": mode})
		return
	case http.MethodPost:
		var body struct {
			Mode string `json:"mode"`
		}
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		m := strings.ToLower(strings.TrimSpace(body.Mode))
		if m != "sync" && m != "save" {
			http.Error(w, "invalid mode", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.state.Mode = m
		s.state.UpdatedAt = time.Now()
		s.mu.Unlock()
		s.saveState()
		// wake scheduler in case scheduling logic cares about mode
		select {
		case s.schedulerCh <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

// SwapOutcome groups results from performSwap so callers can inspect mapping and per-player results
type SwapOutcome struct {
	Mapping         map[string]string
	Results         map[string]string
	DownloadResults map[string]string
}

// performSwap executes the server-side orchestration for a single swap round.
// It is safe to call concurrently; it copies state as needed and only holds the
// mutex for short critical sections when reading/updating shared state.
func (s *Server) performSwap() (*SwapOutcome, error) {
	// dispatch based on server mode
	s.mu.Lock()
	mode := s.state.Mode
	s.mu.Unlock()

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "save":
		return s.performSwapSave()
	case "sync", "":
		return s.performSwapSync()
	default:
		return nil, fmt.Errorf("unknown swap mode: %s", mode)
	}
}

// performSwapSync implements the "sync" mode: all players play the same game and swap
// simultaneously. No saves are uploaded/downloaded; clients just switch current game.
func (s *Server) performSwapSync() (*SwapOutcome, error) {
	s.mu.Lock()
	// basic sanity
	players := []string{}
	for name := range s.state.Players {
		players = append(players, name)
	}
	games := append([]string{}, s.state.Games...)
	s.mu.Unlock()
	if len(players) == 0 || len(games) == 0 {
		return nil, fmt.Errorf("need players and games configured")
	}

	// pick a single game to set for all players; simple rotate using time-based seed
	// choose an index to rotate to so game changes each swap
	idx := rand.Intn(len(games))
	chosen := games[idx]

	mapping := make(map[string]string)
	for _, p := range players {
		mapping[p] = chosen
	}

	// notify clients to switch to chosen game (clients will not upload/download)
	results := make(map[string]string)
	for player, game := range mapping {
		cmdID := fmt.Sprintf("sync-%d-%s", time.Now().UnixNano(), player)
		cmd := types.Command{Cmd: types.CmdSwap, ID: cmdID, Payload: map[string]string{"game": game, "mode": "sync"}}
		res, err := s.sendAndWait(player, cmd, 15*time.Second)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				results[player] = "timeout"
			} else {
				results[player] = "send_failed: " + err.Error()
			}
			continue
		}
		results[player] = res
	}

	// persist updated mapping to state: update Players[*].Current
	s.mu.Lock()
	for p, g := range mapping {
		pl := s.state.Players[p]
		pl.Current = g
		s.state.Players[p] = pl
	}
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	s.saveState()

	return &SwapOutcome{Mapping: mapping, Results: results, DownloadResults: map[string]string{}}, nil

}

// performSwapSave implements the existing save-swap orchestration (previous performSwap)
func (s *Server) performSwapSave() (*SwapOutcome, error) {
	// original logic moved here
	s.mu.Lock()
	// basic sanity: must have players and games
	players := []string{}
	for name := range s.state.Players {
		players = append(players, name)
	}
	games := append([]string{}, s.state.Games...)
	s.mu.Unlock()
	if len(players) == 0 || len(games) == 0 {
		return nil, fmt.Errorf("need players and games configured")
	}

	// simple shuffle mapping: rotate games among players (round-robin)
	mapping := make(map[string]string) // player -> game
	for i, p := range players {
		mapping[p] = games[i%len(games)]
	}

	// Step 1: send swap command to each player (they will save+upload and then switch)
	// Use unique command IDs so we can wait for ack per-client
	results := make(map[string]string)
	for player, game := range mapping {
		cmdID := fmt.Sprintf("swap-%d-%s", time.Now().UnixNano(), player)
		cmd := types.Command{Cmd: types.CmdSwap, ID: cmdID, Payload: map[string]string{"game": game}}
		res, err := s.sendAndWait(player, cmd, 20*time.Second)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				results[player] = "timeout"
			} else {
				results[player] = "send_failed: " + err.Error()
			}
			continue
		}
		results[player] = res
	}

	// Step 2: if any swap failed, return results and abort download phase
	failed := false
	for _, r := range results {
		if r != "ack" {
			failed = true
			break
		}
	}
	if failed {
		return &SwapOutcome{Mapping: mapping, Results: results, DownloadResults: map[string]string{}}, nil
	}

	// Step 3: instruct each player to download the save they should receive.
	// For simplicity we assume uploaded save filename is <game>.state under uploader's folder.
	downloadResults := make(map[string]string)
	for player, game := range mapping {
		owner := ""
		filename := game + ".state"
		// try index.json for latest uploader matching this game or filename
		indexPath := filepath.Join("./saves", "index.json")
		if b, err := os.ReadFile(indexPath); err == nil {
			var idx []SaveIndexEntry
			if err := json.Unmarshal(b, &idx); err == nil {
				var bestAt int64
				for _, e := range idx {
					if e.Game == game || e.File == filename {
						if e.At > bestAt {
							bestAt = e.At
							owner = e.Player
						}
					}
				}
			}
		}
		if owner == "" {
			// fallback: derive from previous Players current_game
			s.mu.Lock()
			for pname, p := range s.state.Players {
				if p.Current == game {
					owner = pname
					break
				}
			}
			s.mu.Unlock()
		}
		if owner == "" {
			owner = player
		}

		cmdID := fmt.Sprintf("dl-%d-%s", time.Now().UnixNano(), player)
		cmd := types.Command{Cmd: types.CmdDownloadSave, ID: cmdID, Payload: map[string]string{"player": owner, "file": filename}}
		if err := s.sendToPlayer(player, cmd); err != nil {
			downloadResults[player] = "send_failed: " + err.Error()
			continue
		}
		res, err := s.waitForResult(cmdID, 30*time.Second)
		if err != nil {
			downloadResults[player] = "timeout"
		} else {
			downloadResults[player] = res
		}
	}

	// persist updated mapping to state: update Players[*].Current
	s.mu.Lock()
	for p, g := range mapping {
		pl := s.state.Players[p]
		pl.Current = g
		s.state.Players[p] = pl
	}
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	s.saveState()

	return &SwapOutcome{Mapping: mapping, Results: results, DownloadResults: downloadResults}, nil

}

func NewServer(stateFile string) *Server {
	s := &Server{
		state: types.ServerState{
			Running:     false,
			SwapEnabled: true,
			Mode:        "sync",
			Games:       []string{},
			MainGames:   []types.GameEntry{},
			Players:     map[string]types.Player{},
			UpdatedAt:   time.Now(),
			// Min/Max interval may be set later; defaulting is handled in scheduler
		},
		conns:       make(map[*websocket.Conn]*wsClient),
		players:     make(map[string]*wsClient),
		upgrader:    websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		stateFile:   stateFile,
		pending:     make(map[string]chan string),
		ephemeral:   make(map[string]string),
		schedulerCh: make(chan struct{}, 1),
	}
	s.loadState()
	// ensure files dir exists for game uploads/assets
	_ = os.MkdirAll("./files", 0755)
	// seed math/rand so per-cycle randomized intervals vary across restarts
	// As of Go 1.20 the package-level random source is automatically
	// seeded at program startup. Calling rand.Seed is deprecated and
	// unnecessary; we rely on the runtime-seeded default Source.

	// ensure saves index exists or rebuild it from disk
	s.reindexSaves()
	// start scheduler loop
	go s.schedulerLoop()
	return s
}

func (s *Server) loadState() {
	// Read state file into a temporary object first, validate, then swap.
	f, err := os.Open(s.stateFile)
	if err != nil {
		log.Printf("state file not found, using defaults: %v", err)
		return
	}
	defer f.Close()

	var tmp types.ServerState
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tmp); err != nil {
		log.Printf("failed to decode state file %s: %v", s.stateFile, err)
		return
	}

	// Basic validation and defaulting
	// If min/max not present, leave zero; scheduler will use sensible fallback
	if tmp.Games == nil {
		tmp.Games = []string{}
	}
	if tmp.MainGames == nil {
		tmp.MainGames = []types.GameEntry{}
	}
	if tmp.Players == nil {
		tmp.Players = map[string]types.Player{}
	}

	// Prevent a zero UpdatedAt from causing surprises
	if tmp.UpdatedAt.IsZero() {
		tmp.UpdatedAt = time.Now()
	}

	// Swap into live state under lock
	s.mu.Lock()
	s.state = tmp
	s.mu.Unlock()
	log.Printf("loaded state from %s", s.stateFile)
}

func (s *Server) saveState() error {
	// Copy state under lock to avoid holding the lock during disk IO
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()

	dir := filepath.Dir(s.stateFile)
	if dir == "" || dir == "." {
		dir = "."
	}
	// create a temp file in the same directory to ensure atomic rename
	tmpFile, err := os.CreateTemp(dir, "state-*.tmp")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(tmpFile)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&st); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		// best-effort; continue to rename below
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return err
	}
	// rename into place
	if err := os.Rename(tmpFile.Name(), s.stateFile); err != nil {
		os.Remove(tmpFile.Name())
		return err
	}
	return nil
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	// serve web/index.html
	http.ServeFile(w, r, "./web/index.html")
}

// serve static assets under /static/
// (files in ./web/static)
// Note: currently external htmx used from CDN

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	// naive file server for now
	http.StripPrefix("/files/", http.FileServer(http.Dir("./files"))).ServeHTTP(w, r)
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
	defer file.Close()
	dstDir := "./files"
	os.MkdirAll(dstDir, 0755)
	dstPath := filepath.Join(dstDir, filepath.Base(header.Filename))
	out, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, "create file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, "write file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte("ok"))
}

// handleFilesList returns a JSON list of files under ./files
func (s *Server) handleFilesList(w http.ResponseWriter, r *http.Request) {
	type fileInfo struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	files := []fileInfo{}
	filepath.Walk("./files", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel("./files", p)
		files = append(files, fileInfo{Name: rel, Size: info.Size()})
		return nil
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// handleBizhawkFilesZip serves a BizhawkFiles.zip either by serving an existing
// ./web/BizhawkFiles.zip or by streaming a zip built on-the-fly from
// ./web/BizhawkFiles directory. This keeps server responses atomic and avoids
// requiring a prebuilt archive in the repo.
func (s *Server) handleBizhawkFilesZip(w http.ResponseWriter, r *http.Request) {
	// prefer prebuilt zip if present
	zipPath := filepath.Join("./web", "BizhawkFiles.zip")
	if fi, err := os.Stat(zipPath); err == nil && !fi.IsDir() {
		// serve the file directly with appropriate headers
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=BizhawkFiles.zip")
		http.ServeFile(w, r, zipPath)
		return
	}

	// otherwise, stream a zip built from ./web/BizhawkFiles
	dir := filepath.Join("./web", "BizhawkFiles")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		http.Error(w, "BizhawkFiles not found", http.StatusNotFound)
		return
	}

	// set headers for streaming zip
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=BizhawkFiles.zip")

	// create a zip writer that writes to the response
	if err := zipDir(dir, w); err != nil {
		log.Printf("failed to stream BizhawkFiles.zip: %v", err)
		// if headers already written, cannot change status; attempt to write error body
		http.Error(w, "failed to create zip", http.StatusInternalServerError)
		return
	}
}

// zipDir writes a zip archive of srcDir to the provided writer.
func zipDir(srcDir string, w io.Writer) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// compute archive name relative to srcDir
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		// skip the root directory entry
		if rel == "." {
			return nil
		}
		// ensure forward slashes in zip entries
		name := filepath.ToSlash(rel)
		if info.IsDir() {
			// add a directory entry (optional)
			_, err := zw.Create(name + "/")
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
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

// handleStateJSON returns the server state as JSON
func (s *Server) handleStateJSON(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	// return the ServerState directly at top-level so the web UI can access
	// fields like min_interval_secs, max_interval_secs, running, etc.
	st := s.state
	// Note: ephemeral statuses are currently not consumed by the UI; if needed
	// we can include them under a separate field later.
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(st)
}

// reindexSaves scans ./saves and writes ./saves/index.json if missing or unreadable
func (s *Server) reindexSaves() {
	idxPath := filepath.Join("./saves", "index.json")
	entries := []SaveIndexEntry{}
	// walk saves dir and collect entries
	_ = filepath.Walk("./saves", func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel("./saves", p)
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		if len(parts) == 2 {
			entries = append(entries, SaveIndexEntry{Player: parts[0], File: parts[1], Size: info.Size(), At: info.ModTime().Unix(), Game: strings.TrimSuffix(parts[1], ".state")})
		}
		return nil
	})

	// ensure directory exists
	if err := os.MkdirAll(filepath.Dir(idxPath), 0755); err != nil {
		return
	}

	// write atomically even for empty list
	ib, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	tmp := idxPath + ".tmp"
	if err := os.WriteFile(tmp, ib, 0644); err != nil {
		return
	}
	_ = os.Rename(tmp, idxPath)
}

// handleSavesList returns a JSON listing of saves under ./saves/<player> directories
func (s *Server) handleSavesList(w http.ResponseWriter, r *http.Request) {
	type SaveInfo struct {
		Player string `json:"player"`
		File   string `json:"file"`
		Size   int64  `json:"size"`
	}
	// prefer using an index file if present for atomic, consistent listings
	indexPath := filepath.Join("./saves", "index.json")
	if b, err := os.ReadFile(indexPath); err == nil {
		var idx []SaveInfo
		if err := json.Unmarshal(b, &idx); err == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(idx)
			return
		}
		// fallthrough to directory walk on unmarshal error
	}

	saves := []SaveInfo{}
	filepath.Walk("./saves", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel("./saves", p)
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		if len(parts) == 2 {
			saves = append(saves, SaveInfo{Player: parts[0], File: parts[1], Size: info.Size()})
		}
		return nil
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(saves)
}

// apiGames: GET returns games, POST accepts JSON body {"games":[...]}
func (s *Server) apiGames(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.Lock()
		// Return both the main catalog and the active games list so the UI
		// can edit either. Keep keys "main_games" and "games".
		resp := map[string]any{"main_games": s.state.MainGames, "games": s.state.Games}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}
	if r.Method == http.MethodPost {
		// Accept either legacy {"games": [...]} or new {"main_games": [...], "games": [...]} payloads.
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		// update MainGames if provided
		if mg, ok := raw["main_games"]; ok {
			// decode into []types.GameEntry
			b, _ := json.Marshal(mg)
			var entries []types.GameEntry
			if err := json.Unmarshal(b, &entries); err == nil {
				s.state.MainGames = entries
			}
		}
		// update active games if provided (legacy key "games")
		if g, ok := raw["games"]; ok {
			b, _ := json.Marshal(g)
			var games []string
			if err := json.Unmarshal(b, &games); err == nil {
				s.state.Games = games
			}
		}
		s.state.UpdatedAt = time.Now()
		s.mu.Unlock()
		s.saveState()
		// broadcast state update to clients
		s.broadcast(types.Command{Cmd: types.CmdStateUpdate, Payload: map[string]any{"updated_at": s.state.UpdatedAt}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
		// also notify clients of the updated games/main_games so they can download missing files
		s.broadcast(types.Command{Cmd: types.CmdGamesUpdate, Payload: map[string]any{"games": s.state.Games, "main_games": s.state.MainGames}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
		w.Write([]byte("ok"))
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// apiInterval: GET/POST to view or set interval seconds
func (s *Server) apiInterval(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.Lock()
		minv := s.state.MinIntervalSecs
		maxv := s.state.MaxIntervalSecs
		s.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"min_interval_secs": minv, "max_interval_secs": maxv})
		return
	}
	if r.Method == http.MethodPost {
		var b struct {
			MinInterval int `json:"min_interval_secs"`
			MaxInterval int `json:"max_interval_secs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		if b.MinInterval != 0 {
			s.state.MinIntervalSecs = b.MinInterval
		}
		if b.MaxInterval != 0 {
			s.state.MaxIntervalSecs = b.MaxInterval
		}
		s.state.UpdatedAt = time.Now()
		s.mu.Unlock()
		s.saveState()
		s.broadcast(types.Command{Cmd: types.CmdStateUpdate, Payload: map[string]any{"updated_at": s.state.UpdatedAt}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
		w.Write([]byte("ok"))
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// apiSwapPlayer: POST {player:..., game:...}
func (s *Server) apiSwapPlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Player string `json:"player"`
		Game   string `json:"game"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	cmdID := fmt.Sprintf("swap-%d-%s", time.Now().UnixNano(), b.Player)
	cmd := types.Command{Cmd: types.CmdSwap, ID: cmdID, Payload: map[string]string{"game": b.Game}}
	res, err := s.sendAndWait(b.Player, cmd, 20*time.Second)
	if err != nil {
		if errors.Is(err, ErrTimeout) {
			http.Error(w, "timeout", http.StatusGatewayTimeout)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"result": res})
}

// apiRemovePlayer: POST {player: ...}
func (s *Server) apiRemovePlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Player string `json:"player"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if b.Player == "" {
		http.Error(w, "missing player", http.StatusBadRequest)
		return
	}

	// remove player from state and close any connection
	s.mu.Lock()
	// remove from state.Players
	delete(s.state.Players, b.Player)
	// if there's a ws client for this player, close it and remove from maps
	if cl, ok := s.players[b.Player]; ok {
		// find websocket.Conn that maps to this wsClient in s.conns and close it
		for c, client := range s.conns {
			if client == cl {
				// remove from conns map and close connection
				delete(s.conns, c)
				// close underlying websocket; best-effort
				_ = c.Close()
				break
			}
		}
		delete(s.players, b.Player)
	}
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()

	// persist and broadcast update to remaining clients
	s.saveState()
	s.broadcast(types.Command{Cmd: types.CmdStateUpdate, Payload: map[string]any{"updated_at": s.state.UpdatedAt}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
}

// apiSwapAllToGame: POST {game:...}
func (s *Server) apiSwapAllToGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var b struct {
		Game string `json:"game"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	players := []string{}
	for name := range s.state.Players {
		players = append(players, name)
	}
	s.mu.Unlock()
	results := map[string]string{}
	for _, p := range players {
		cmdID := fmt.Sprintf("swap-%d-%s", time.Now().UnixNano(), p)
		cmd := types.Command{Cmd: types.CmdSwap, ID: cmdID, Payload: map[string]string{"game": b.Game}}
		res, err := s.sendAndWait(p, cmd, 20*time.Second)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				results[p] = "timeout"
			} else {
				results[p] = "send_failed: " + err.Error()
			}
			continue
		}
		results[p] = res
	}
	json.NewEncoder(w).Encode(results)
}

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
	// choose filename: prefer explicit form field "filename" if provided, else header.Filename, else game
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
	// safe base name
	fname = filepath.Base(fname)

	// write to a temp file in the same dir and rename atomically
	tmp := filepath.Join(dir, fname+".tmp")
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		http.Error(w, "create tmp: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// enforce a maximum save size to avoid OOM/disk abuse
	lr := &io.LimitedReader{R: file, N: maxSaveSize}
	if _, err := io.Copy(out, lr); err != nil {
		out.Close()
		os.Remove(tmp)
		http.Error(w, "write tmp: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if lr.N == 0 {
		// reached limit; reject
		out.Close()
		os.Remove(tmp)
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	out.Close()
	dst := filepath.Join(dir, fname)
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		http.Error(w, "rename: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// update index.json under ./saves/index.json atomically
	indexPath := filepath.Join("./saves", "index.json")
	type SaveIndexEntry struct {
		Player string `json:"player"`
		File   string `json:"file"`
		Size   int64  `json:"size"`
		At     int64  `json:"at"`
		Game   string `json:"game"`
	}
	var idx []SaveIndexEntry
	s.mu.Lock()
	if b, err := os.ReadFile(indexPath); err == nil {
		_ = json.Unmarshal(b, &idx)
	}
	// remove any existing entry for this player/file
	newIdx := make([]SaveIndexEntry, 0, len(idx)+1)
	for _, e := range idx {
		if !(e.Player == player && e.File == fname) {
			newIdx = append(newIdx, e)
		}
	}
	fi, _ := os.Stat(dst)
	newIdx = append(newIdx, SaveIndexEntry{Player: player, File: fname, Size: fi.Size(), At: time.Now().Unix(), Game: game})
	// write index to a temp file and rename
	tmpIndex := indexPath + ".tmp"
	ib, _ := json.MarshalIndent(newIdx, "", "  ")
	if err := os.WriteFile(tmpIndex, ib, 0644); err == nil {
		os.Rename(tmpIndex, indexPath)
	}
	s.mu.Unlock()

	// rebuild saves index for consistency
	go s.reindexSaves()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "ok", "file": dst})
}

// handleSaveServe serves files under ./saves via /save/<player>/<file>
func (s *Server) handleSaveServe(w http.ResponseWriter, r *http.Request) {
	// trim prefix /save/
	rel := strings.TrimPrefix(r.URL.Path, "/save/")
	if rel == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	// prevent path traversal; treat URL path components (use '/')
	rel = strings.TrimPrefix(rel, "/")
	rel = path.Clean(rel)
	parts := strings.SplitN(rel, "/", 2)
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	// serve the file from ./saves/<player>/<file>
	p := filepath.Join("./saves", parts[0], parts[1])
	http.ServeFile(w, r, p)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	// register connection with empty client until hello
	client := &wsClient{conn: c, sendCh: make(chan types.Command, 8)}
	s.mu.Lock()
	s.conns[c] = client
	s.mu.Unlock()

	// set read limits and ping/pong handling
	c.SetReadLimit(1024 * 16)
	c.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.SetPongHandler(func(string) error {
		c.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// per-connection write pump using client.sendCh
	var writeWG sync.WaitGroup
	writeWG.Add(1)
	go func() {
		// writer goroutine
		ticker := time.NewTicker(30 * time.Second)
		defer func() { ticker.Stop(); writeWG.Done() }()
		for {
			select {
			case cmd, ok := <-client.sendCh:
				c.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if !ok {
					// channel closed
					c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					return
				}
				if err := c.WriteJSON(cmd); err != nil {
					log.Printf("write json err: %v", err)
					return
				}
			case <-ticker.C:
				c.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
	// cleanup
	defer func() {
		// close send channel then wait for writer
		close(client.sendCh)
		writeWG.Wait()
		s.mu.Lock()
		// mark player disconnected if we know the name
		if cl, ok := s.conns[c]; ok {
			name := ""
			// find name by reverse map
			for n, pc := range s.players {
				if pc == cl {
					name = n
					break
				}
			}
			if name != "" {
				pl := s.state.Players[name]
				pl.Connected = false
				s.state.Players[name] = pl
				delete(s.players, name)
			}
			delete(s.conns, c)
		}
		s.mu.Unlock()
		s.saveState()
		c.Close()
	}()
	// read loop
	for {
		var cmd types.Command
		if err := c.ReadJSON(&cmd); err != nil {
			log.Printf("read: %v", err)
			break
		}
		log.Printf("received cmd from client: %s id=%s", cmd.Cmd, cmd.ID)

		// If client is sending ack/nack for a server-sent command, record result
		if cmd.Cmd == types.CmdAck || cmd.Cmd == types.CmdNack {
			s.mu.Lock()
			ch, ok := s.pending[cmd.ID]
			if ok {
				// send the raw payload or status to waiter
				if cmd.Cmd == types.CmdAck {
					select {
					case ch <- "ack":
					default:
					}
				} else {
					// prefer to marshal the whole payload for richer diagnostics
					var reason string = "nack"
					if cmd.Payload != nil {
						if b, err := json.Marshal(cmd.Payload); err == nil {
							reason = "nack|" + string(b)
						} else {
							// fallback: try to extract reason string
							if pl, ok := cmd.Payload.(map[string]any); ok {
								if rstr, ok := pl["reason"].(string); ok {
									reason = "nack|" + rstr
								}
							}
						}
					}
					log.Printf("received nack id=%s payload=%+v", cmd.ID, cmd.Payload)
					select {
					case ch <- reason:
					default:
					}
				}
				close(ch)
				delete(s.pending, cmd.ID)
			}
			s.mu.Unlock()
			continue
		}

		// status update messages from client: {cmd: "status", payload:{"status":"uploading"}}
		if cmd.Cmd == types.CmdStatus || cmd.Cmd == types.CmdStateUpdate {
			// map client -> player name by scanning players map
			s.mu.Lock()
			var pname string
			for n, pc := range s.players {
				if pc == client {
					pname = n
					break
				}
			}
			if pname != "" {
				if pl, ok := cmd.Payload.(map[string]any); ok {
					if st, ok := pl["status"].(string); ok {
						s.ephemeral[pname] = st
					}
				}
			}
			s.mu.Unlock()
			continue
		}

		// client reports results of games download (sent after games_update)
		if cmd.Cmd == types.CmdGamesUpdateAck {
			// payload: {"has_files": bool}
			s.mu.Lock()
			var pname string
			for n, pc := range s.players {
				if pc == client {
					pname = n
					break
				}
			}
			if pname != "" {
				if pl, ok := cmd.Payload.(map[string]any); ok {
					if hf, ok := pl["has_files"].(bool); ok {
						p := s.state.Players[pname]
						p.HasFiles = hf
						s.state.Players[pname] = p
						s.state.UpdatedAt = time.Now()
					}
				}
			}
			s.mu.Unlock()
			// persist and notify clients so UI can reflect updated HasFiles
			s.saveState()
			s.broadcast(types.Command{Cmd: types.CmdStateUpdate, Payload: map[string]any{"updated_at": s.state.UpdatedAt}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
			continue
		}
		// handle hello to register player
		if cmd.Cmd == types.CmdHello {
			if pl, ok := cmd.Payload.(map[string]any); ok {
				name := "player"
				if v, ok := pl["name"].(string); ok {
					name = v
				}
				s.mu.Lock()
				s.state.Players[name] = types.Player{Name: name, Connected: true}
				s.conns[c] = client
				s.players[name] = client
				s.mu.Unlock()
				s.saveState()

				// send current games/main_games to the newly connected client so it can
				// download any missing files immediately
				s.mu.Lock()
				payload := map[string]any{"games": s.state.Games, "main_games": s.state.MainGames}
				s.mu.Unlock()
				select {
				case client.sendCh <- types.Command{Cmd: types.CmdGamesUpdate, Payload: payload, ID: fmt.Sprintf("%d", time.Now().UnixNano())}:
				default:
					// if send queue is full, drop the notification; client can fetch /state.json later
				}
			}
			continue
		}

		// for other messages from client, just log for now
		log.Printf("client message: %+v", cmd)
	}
}

// helper: send a command to a specific player (by name)
func (s *Server) sendToPlayer(player string, cmd types.Command) error {
	s.mu.Lock()
	client, ok := s.players[player]
	s.mu.Unlock()
	if !ok || client == nil {
		return fmt.Errorf("no connection for player %s", player)
	}
	// enqueue to send channel; do a non-blocking send to avoid blocking server
	select {
	case client.sendCh <- cmd:
		return nil
	default:
		return fmt.Errorf("send queue full for player %s", player)
	}
}

// helper: wait for ack/nack on a command ID with timeout
func (s *Server) waitForResult(cmdID string, timeout time.Duration) (string, error) {
	ch := make(chan string, 1)
	s.mu.Lock()
	s.pending[cmdID] = ch
	s.mu.Unlock()
	select {
	case res := <-ch:
		return res, nil
	case <-time.After(timeout):
		// timeout, cleanup
		s.mu.Lock()
		delete(s.pending, cmdID)
		s.mu.Unlock()
		return "", fmt.Errorf("timeout waiting for result %s", cmdID)
	}
}

// sendAndWait registers a pending channel for the given command ID, enqueues
// the command to the target player, and waits for ack/nack with timeout.
// Returns the raw result string ("ack" or "nack|reason") or an error.
func (s *Server) sendAndWait(player string, cmd types.Command, timeout time.Duration) (string, error) {
	ch := make(chan string, 1)
	s.mu.Lock()
	s.pending[cmd.ID] = ch
	s.mu.Unlock()

	// ensure cleanup on return
	defer func() {
		s.mu.Lock()
		delete(s.pending, cmd.ID)
		s.mu.Unlock()
	}()

	if err := s.sendToPlayer(player, cmd); err != nil {
		return "", err
	}

	select {
	case res := <-ch:
		return res, nil
	case <-time.After(timeout):
		return "", ErrTimeout
	}
}

func (s *Server) broadcast(cmd types.Command) {
	s.mu.Lock()
	clients := make([]*wsClient, 0, len(s.players))
	log.Printf("broadcasting command %s to %d clients", cmd.Cmd, len(s.players))
	for _, cl := range s.players {
		clients = append(clients, cl)
	}
	s.mu.Unlock()
	for _, cl := range clients {
		select {
		case cl.sendCh <- cmd:
		default:
			// drop if client queue is full
		}
	}
}

func (s *Server) apiStart(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.state.Running = true
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	s.saveState()
	s.broadcast(types.Command{Cmd: types.CmdStart, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	// wake scheduler to pick up new running state
	select {
	case s.schedulerCh <- struct{}{}:
	default:
	}
	w.Write([]byte("ok"))
}

func (s *Server) apiPause(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.state.Running = false
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	s.saveState()
	s.broadcast(types.Command{Cmd: types.CmdPause, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	// wake scheduler to stop sleeping
	select {
	case s.schedulerCh <- struct{}{}:
	default:
	}
	w.Write([]byte("ok"))
}

func (s *Server) apiReset(w http.ResponseWriter, r *http.Request) {
	// reset state except players
	s.mu.Lock()
	s.state.Games = []string{}
	s.state.Running = false
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	s.saveState()
	s.broadcast(types.Command{Cmd: types.CmdReset, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	w.Write([]byte("ok"))
}

func (s *Server) apiClearSaves(w http.ResponseWriter, r *http.Request) {
	// server-side: move ./saves to a trash folder and recreate empty index.json
	savesDir := "./saves"
	if _, err := os.Stat(savesDir); err == nil {
		trash := fmt.Sprintf("%s.trash.%d", savesDir, time.Now().Unix())
		_ = os.Rename(savesDir, trash)
	}
	// recreate empty saves dir and index
	_ = os.MkdirAll(savesDir, 0755)
	indexPath := filepath.Join(savesDir, "index.json")
	_ = os.WriteFile(indexPath, []byte("[]"), 0644)

	// notify clients to clear saves as well
	s.broadcast(types.Command{Cmd: types.CmdClearSaves, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	w.Write([]byte("ok"))
}

func (s *Server) apiToggleSwaps(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.state.SwapEnabled = !s.state.SwapEnabled
	// if disabling automatic swaps, clear the scheduled next swap time so UI shows "--"
	if !s.state.SwapEnabled {
		s.state.NextSwapAt = 0
	}
	s.state.UpdatedAt = time.Now()
	s.mu.Unlock()
	s.saveState()
	// include next_swap_at in payload so clients can immediately update the display
	s.broadcast(types.Command{Cmd: types.CmdToggleSwaps, Payload: map[string]any{"enabled": s.state.SwapEnabled, "next_swap_at": s.state.NextSwapAt}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})
	// wake scheduler to re-evaluate
	select {
	case s.schedulerCh <- struct{}{}:
	default:
	}
	w.Write([]byte("ok"))
}

// schedulerLoop runs in background and schedules swaps when running and swap enabled.
// It picks a random interval between MinIntervalSecs and MaxIntervalSecs (inclusive)
// when both are set, otherwise uses MinIntervalSecs/MaxIntervalSecs or a default.
func (s *Server) schedulerLoop() {
	for {
		// wait until running and swaps enabled
		s.mu.Lock()
		running := s.state.Running
		enabled := s.state.SwapEnabled
		s.mu.Unlock()
		if !running || !enabled {
			// block until signaled to re-evaluate
			<-s.schedulerCh
			continue
		}

		// determine interval: prefer random between MinIntervalSecs and MaxIntervalSecs
		// if both are set and valid, otherwise use MinIntervalSecs if set, else fallback to 300s
		s.mu.Lock()
		minv := s.state.MinIntervalSecs
		maxv := s.state.MaxIntervalSecs
		s.mu.Unlock()

		var interval int
		if minv > 0 && maxv > 0 && maxv >= minv {
			interval = minv + rand.Intn(maxv-minv+1)
		} else if minv > 0 {
			interval = minv
		} else if maxv > 0 {
			interval = maxv
		} else {
			interval = 300 // default
		}

		// compute next swap time and persist
		nextAt := time.Now().Add(time.Duration(interval) * time.Second).Unix()
		s.mu.Lock()
		s.state.NextSwapAt = nextAt
		s.state.UpdatedAt = time.Now()
		s.mu.Unlock()
		s.saveState()
		s.broadcast(types.Command{Cmd: types.CmdStateUpdate, Payload: map[string]any{"next_swap_at": nextAt, "updated_at": s.state.UpdatedAt}, ID: fmt.Sprintf("%d", time.Now().UnixNano())})

		// wait for interval but wake early if signaled
		timer := time.NewTimer(time.Duration(interval) * time.Second)
		select {
		case <-timer.C:
			// time to perform swap
		case <-s.schedulerCh:
			// re-evaluate immediately
			if !timer.Stop() {
				<-timer.C
			}
			continue
		}

		// double-check running/enabled before invoking
		s.mu.Lock()
		if !(s.state.Running && s.state.SwapEnabled) {
			s.mu.Unlock()
			continue
		}
		s.mu.Unlock()

		// perform swap in background so scheduler can continue
		go func() {
			_, err := s.performSwap()
			if err != nil {
				log.Printf("performSwap error: %v", err)
			}
			// wake scheduler to pick next interval immediately
			select {
			case s.schedulerCh <- struct{}{}:
			default:
			}
		}()
	}
}
