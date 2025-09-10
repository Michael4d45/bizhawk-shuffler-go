package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michael4d45/bizshuffle/internal/server/p2p"
	"github.com/michael4d45/bizshuffle/internal/types"
)

// Server encapsulates all state and connected websocket clients.
type Server struct {
	mu          sync.Mutex
	state       types.ServerState
	conns       map[*websocket.Conn]*wsClient
	players     map[string]*wsClient
	upgrader    websocket.Upgrader
	pending     map[string]chan string
	schedulerCh chan struct{}
	broadcaster *DiscoveryBroadcaster
	tracker     *p2p.SaveTracker
}

// ErrTimeout is exported so callers can detect timeout waiting for a client ack/nack.
var ErrTimeout = fmt.Errorf("timeout waiting for result")

// New creates and initializes a Server, loading state and starting the scheduler.
func New() *Server {
	s := &Server{
		state: types.ServerState{
			Running:           false,
			SwapEnabled:       true,
			Mode:              types.GameModeSync,
			MainGames:         []types.GameEntry{},
			Plugins:           make(map[string]types.Plugin),
			GameSwapInstances: []types.GameSwapInstance{},
			Games:             []string{},
			Players:           map[string]types.Player{},
			UpdatedAt:         time.Now(),
			MinIntervalSecs:   5,
			MaxIntervalSecs:   300,
		},
		conns:       make(map[*websocket.Conn]*wsClient),
		players:     make(map[string]*wsClient),
		upgrader:    websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		pending:     make(map[string]chan string),
		schedulerCh: make(chan struct{}, 1),
	}
	s.loadState()
	_ = os.MkdirAll("./files", 0755)
	_ = os.MkdirAll("./saves", 0755)
	// Initialize tracker (even if P2P disabled we can collect announces for debugging)
	s.tracker = p2p.NewSaveTracker(2*time.Minute, 30*time.Second)
	go s.schedulerLoop()
	return s
}

// RegisterRoutes attaches all HTTP handlers to the provided mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/", s.handleAdmin)
	mux.HandleFunc("/api/start", s.apiStart)
	mux.HandleFunc("/api/pause", s.apiPause)
	mux.HandleFunc("/api/reset", s.apiReset)
	mux.HandleFunc("/api/clear_saves", s.apiClearSaves)
	mux.HandleFunc("/api/toggle_swaps", s.apiToggleSwaps)
	mux.HandleFunc("/api/do_swap", s.apiDoSwap)
	mux.HandleFunc("/api/mode/setup", s.apiModeSetup)
	mux.HandleFunc("/api/mode", s.apiMode)
	mux.HandleFunc("/files/", s.handleFiles)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/files/list.json", s.handleFilesList)
	mux.HandleFunc("/api/BizhawkFiles.zip", s.handleBizhawkFilesZip)
	mux.HandleFunc("/state.json", s.handleStateJSON)
	mux.HandleFunc("/api/games", s.apiGames)
	mux.HandleFunc("/api/interval", s.apiInterval)
	mux.HandleFunc("/api/swap_player", s.apiSwapPlayer)
	mux.HandleFunc("/api/remove_player", s.apiRemovePlayer)
	mux.HandleFunc("/api/swap_all_to_game", s.apiSwapAllToGame)
	// Plugin management routes
	mux.HandleFunc("/api/plugins", s.handlePluginsList)
	mux.HandleFunc("/api/plugins/upload", s.handlePluginUpload)
	// Plugin enable/disable routes - these need to handle the plugin name in the URL path
	mux.HandleFunc("/api/plugins/", s.handlePluginAction)
	mux.HandleFunc("/api/message_player", s.apiMessagePlayer)
	mux.HandleFunc("/api/message_all", s.apiMessageAll)
	// Save state management endpoints
	mux.HandleFunc("/save/upload", s.handleSaveUpload)
	mux.HandleFunc("/save/", s.handleSaveDownload)
	// P2P save state endpoints (alpha):
	//   - save-manifest: returns current versions
	//   - save-announce: registers peer (in-memory tracker)
	//   - save-peers: returns peers for an instance (or snapshot summary when none provided)
	//   - save-status: ingests (currently logs/discards) client metrics
	mux.HandleFunc("/api/p2p/save-manifest", s.handleP2PManifest)
	mux.HandleFunc("/api/p2p/save-announce", s.handleP2PAnnounce)
	mux.HandleFunc("/api/p2p/save-peers", s.handleP2PPeers)
	mux.HandleFunc("/api/p2p/save-status", s.handleP2PStatus)
}

// writeJSON is a small helper for consistent JSON responses.
func writeJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	return enc.Encode(v)
}

// handleP2PManifest returns a manifest with current save state versions.
func (s *Server) handleP2PManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	manifestVersion := s.state.SaveStateManifestVersion
	pieceSize := s.state.SaveStatePieceSize
	instances := append([]types.GameSwapInstance(nil), s.state.GameSwapInstances...)
	s.mu.Unlock()
	saves := make([]types.SaveStateVersion, 0, len(instances))
	for _, inst := range instances {
		if inst.SaveHash == "" || inst.SaveSize == 0 {
			continue // skip instances without a versioned save yet
		}
		saves = append(saves, types.SaveStateVersion{
			InstanceID: inst.ID,
			Hash:       inst.SaveHash,
			Size:       inst.SaveSize,
			UpdatedAt:  inst.SaveUpdated,
			PieceSize:  inst.SavePieceLen,
		})
	}
	manifest := types.SaveStateManifest{Version: manifestVersion, Generated: time.Now(), Saves: saves, PieceSize: pieceSize}
	if err := writeJSON(w, manifest); err != nil {
		log.Printf("p2p_manifest write error: %v", err)
	}
}

// handleP2PAnnounce registers a peer's availability. For now we accept and log; no peer store yet.
func (s *Server) handleP2PAnnounce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		PeerID    string                 `json:"peer_id"`
		Instances []p2p.AnnounceInstance `json:"instances"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		log.Printf("[P2P][announce][unexpected] decode error: %v", err)
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()
	remote := r.RemoteAddr
	if body.PeerID == "" {
		log.Printf("[P2P][announce][expected] missing peer_id remote=%s", remote)
		// still 200 to avoid noisy client retries, but log expectation
		_, _ = w.Write([]byte("ok"))
		return
	}
	s.tracker.Announce(body.PeerID, remote, body.Instances)
	log.Printf("[P2P][announce] peer=%s insts=%d remote=%s", body.PeerID, len(body.Instances), remote)
	if _, err := w.Write([]byte("ok")); err != nil {
		log.Printf("[P2P][announce][unexpected] write error: %v", err)
	}
}

// handleP2PPeers returns peer list for an instance. Stub returns empty list until tracker implemented.
func (s *Server) handleP2PPeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	instanceID := r.URL.Query().Get("instance_id")
	var peers []types.PeerInfo
	if instanceID != "" {
		peers = s.tracker.Peers(instanceID)
	} else {
		// aggregate all peers (flatten) for debugging when no instance specified
		peers = s.tracker.Peers("") // will return nil; we will build manual aggregate
	}
	if peers == nil && instanceID == "" {
		// Build a synthetic summary list from tracker snapshot
		snap := s.tracker.Snapshot()
		if err := writeJSON(w, snap); err != nil {
			log.Printf("p2p_peers write error: %v", err)
		}
		return
	}
	if err := writeJSON(w, peers); err != nil {
		log.Printf("p2p_peers write error: %v", err)
	}
}

// handleP2PStatus ingests client metrics. For now we log and return ok.
func (s *Server) handleP2PStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Minimal ingestion: attempt JSON decode into generic map for future metrics aggregation.
	var payload map[string]any
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&payload); err != nil && err != io.EOF {
		log.Printf("[P2P][status][unexpected] decode err=%v", err)
	} else if len(payload) > 0 {
		// Log a compact summary (size only) to avoid flooding logs.
		log.Printf("[P2P][status] received fields=%d", len(payload))
	}
	_ = r.Body.Close()
	if _, err := w.Write([]byte("ok")); err != nil {
		log.Printf("p2p_status write error: %v", err)
	}
}

// RegenerateSaveManifest scans the saves directory and ensures the server state contains
// up-to-date hash/size metadata for each instance that has a corresponding file. It returns
// (changed, error). It increments the manifest version if any instance metadata changed.
// This is a best-effort operation; failures are logged and surfaced as error but do not
// block callers (they typically run it in a goroutine).
func (s *Server) RegenerateSaveManifest() (bool, error) {
	s.mu.Lock()
	instances := append([]types.GameSwapInstance(nil), s.state.GameSwapInstances...)
	pieceSize := s.state.SaveStatePieceSize
	s.mu.Unlock()
	files, err := os.ReadDir("./saves")
	if err != nil {
		return false, fmt.Errorf("read saves dir: %w", err)
	}
	fileMap := make(map[string]os.DirEntry)
	for _, f := range files {
		name := f.Name()
		if len(name) > 6 && name[len(name)-6:] == ".state" {
			id := name[:len(name)-6]
			fileMap[id] = f
		}
	}
	changed := false
	for i, inst := range instances {
		if _, ok := fileMap[inst.ID]; !ok {
			continue
		}
		path := "./saves/" + inst.ID + ".state"
		st, err := os.Stat(path)
		if err != nil {
			log.Printf("[P2P][manifest][expected] stat failed path=%s err=%v", path, err)
			continue
		}
		// If size matches and timestamp not newer than recorded, skip hashing for speed.
		if inst.SaveSize == st.Size() && !st.ModTime().After(inst.SaveUpdated) && inst.SaveHash != "" {
			continue
		}
		// Hash file
		f, err := os.Open(path)
		if err != nil {
			log.Printf("[P2P][manifest][unexpected] open failed path=%s err=%v", path, err)
			continue
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		_ = f.Close()
		if err != nil {
			log.Printf("[P2P][manifest][unexpected] hash copy failed path=%s err=%v", path, err)
			continue
		}
		sum := hex.EncodeToString(h.Sum(nil))
		if sum != inst.SaveHash || n != inst.SaveSize {
			instances[i].SaveHash = sum
			instances[i].SaveSize = n
			instances[i].SaveUpdated = time.Now()
			instances[i].SavePieceLen = pieceSize
			instances[i].FileState = types.FileStateReady
			changed = true
		}
	}
	if changed {
		s.mu.Lock()
		// write back updated slice
		for i := range instances {
			// align to state slice by ID
			for j := range s.state.GameSwapInstances {
				if s.state.GameSwapInstances[j].ID == instances[i].ID {
					s.state.GameSwapInstances[j] = instances[i]
					break
				}
			}
		}
		s.state.SaveStateManifestVersion++
		ver := s.state.SaveStateManifestVersion
		s.state.UpdatedAt = time.Now()
		s.mu.Unlock()
		log.Printf("[P2P][manifest] regeneration updated manifest_version=%d", ver)
	}
	return changed, nil
}

// CurrentManifestVersion returns the latest manifest version (thread-safe).
func (s *Server) CurrentManifestVersion() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.SaveStateManifestVersion
}

// broadcast sends a command to all currently connected players.
func (s *Server) broadcast(cmd types.Command) {
	s.mu.Lock()
	clients := make([]*wsClient, 0, len(s.players))
	for _, cl := range s.players {
		clients = append(clients, cl)
	}
	s.mu.Unlock()
	for _, cl := range clients {
		select {
		case cl.sendCh <- cmd:
		default:
			// drop if queue full
		}
	}
}

// UpdateHostIfChanged sets host in state if different and persists.
func (s *Server) UpdateHostIfChanged(host string) {
	s.mu.Lock()
	if s.state.Host == host {
		s.mu.Unlock()
		return
	}
	s.state.Host = host
	s.state.UpdatedAt = time.Now()
	st := s.state
	s.mu.Unlock()
	if err := s.saveState(); err != nil {
		log.Printf("failed to persist host: %v", err)
	} else {
		log.Printf("host updated to %s", st.Host)
	}
}

// PersistedHost returns the persisted host if present.
func (s *Server) PersistedHost() string { s.mu.Lock(); defer s.mu.Unlock(); return s.state.Host }

// UpdatePortIfChanged sets port in state if different and persists.
func (s *Server) UpdatePortIfChanged(port int) {
	s.mu.Lock()
	if s.state.Port == port {
		s.mu.Unlock()
		return
	}
	s.state.Port = port
	s.state.UpdatedAt = time.Now()
	st := s.state
	s.mu.Unlock()
	if err := s.saveState(); err != nil {
		log.Printf("failed to persist port: %v", err)
	} else {
		log.Printf("port updated to %d", st.Port)
	}
}

// PersistedPort returns the persisted port if present.
func (s *Server) PersistedPort() int { s.mu.Lock(); defer s.mu.Unlock(); return s.state.Port }

// StartBroadcaster initializes and starts the discovery broadcaster
func (s *Server) StartBroadcaster(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.broadcaster != nil {
		return s.broadcaster.Start(ctx)
	}

	// Create default discovery config
	config := types.GetDefaultDiscoveryConfig()

	// Get server info
	host := s.state.Host
	if host == "" {
		host = "127.0.0.1" // fallback
	}
	port := s.state.Port
	if port == 0 {
		port = 8080 // fallback
	}
	serverName := s.GetServerName()

	// Initialize broadcaster
	s.broadcaster = NewDiscoveryBroadcaster(config, host, port, serverName)
	return s.broadcaster.Start(ctx)
}

// StopBroadcaster stops the discovery broadcaster
func (s *Server) StopBroadcaster() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.broadcaster != nil {
		return s.broadcaster.Stop()
	}
	return nil
}

// GetServerName returns a human-readable name for this server
func (s *Server) GetServerName() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "BizShuffle"
	}
	return fmt.Sprintf("%s Server", hostname)
}

func (s *Server) currentPlayer(player string) types.Player {
	playerInfo := s.GetGameForPlayer(player)
	if playerInfo.Game != "" {
		return playerInfo
	}
	handler := s.GetGameModeHandler()
	playerInfo = handler.GetPlayer(player)
	return playerInfo
}
