package serverhost

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michael4d45/bizshuffle/protocol"
)

// Server encapsulates all state and connected websocket clients.
//
// Lock ownership:
//   - connMu: websocket registries (conns, playerClients, adminClients)
//   - mu: server state, pending acks, swap tracking, plugins in memory
//   - liveConns: lock-free snapshot for shutdown socket close
//   - broadcaster: atomic.Pointer, stopped without mu
type Server struct {
	mu                   sync.RWMutex
	connMu               sync.RWMutex
	pendingInstancecount int
	state                protocol.ServerState
	conns                map[*websocket.Conn]*wsClient
	playerClients        map[string]*wsClient
	adminClients         map[string]*wsClient
	upgrader             websocket.Upgrader
	pending              map[string]chan string
	schedulerCh          chan struct{}
	broadcaster          atomic.Pointer[DiscoveryBroadcaster]
	saveChan             chan struct{}
	saveTimer            *time.Timer
	saveMutex            sync.Mutex
	appliedSwapTarget    map[string]string
	swapInFlight         map[string]struct{}
	openInFileManager    func(path string) error // nil: use OS default (explorer/open/xdg-open)
	wsActive             sync.WaitGroup
	shuttingDown         int32
	liveConns            sync.Map // *websocket.Conn -> *wsClient; used for shutdown without s.mu
}

// ErrTimeout is exported so callers can detect timeout waiting for a client ack/nack.
var ErrTimeout = fmt.Errorf("timeout waiting for result")

// New creates and initializes a Server, loading state and starting the scheduler.
func New() *Server {
	s := &Server{
		state: protocol.ServerState{
			Running:             false,
			SwapEnabled:         true,
			Mode:                protocol.GameModeSync,
			MainGames:           []protocol.GameEntry{},
			Plugins:             make(map[string]protocol.Plugin),
			GameSwapInstances:   []protocol.GameSwapInstance{},
			Games:               []string{},
			Players:             map[string]protocol.Player{},
			UpdatedAt:           time.Now(),
			MinIntervalSecs:     5,
			MaxIntervalSecs:     300,
			PreventSameGameSwap: false,
		},
		conns:             make(map[*websocket.Conn]*wsClient),
		playerClients:     make(map[string]*wsClient),
		adminClients:      make(map[string]*wsClient),
		upgrader:          websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		pending:           make(map[string]chan string),
		schedulerCh:       make(chan struct{}, 1),
		saveChan:          make(chan struct{}, 1),
		appliedSwapTarget: make(map[string]string),
		swapInFlight:      make(map[string]struct{}),
	}
	s.loadState()
	_ = os.MkdirAll("./roms", 0755)
	_ = os.MkdirAll("./saves", 0755)
	go s.schedulerLoop()
	go s.startSaver()
	return s
}

// RegisterRoutes attaches all HTTP handlers to the provided mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/", s.handleAdmin)
	mux.HandleFunc("/api/start", s.apiStart)
	mux.HandleFunc("/api/pause", s.apiPause)
	mux.HandleFunc("/api/clear_saves", s.apiClearSaves)
	mux.HandleFunc("/api/toggle_swaps", s.apiToggleSwaps)
	mux.HandleFunc("/api/toggle_countdown", s.apiToggleCountdown)
	mux.HandleFunc("/api/do_swap", s.apiDoSwap)
	mux.HandleFunc("/api/random_swap", s.apiRandomSwapForPlayer)
	mux.HandleFunc("/api/mode/setup", s.apiModeSetup)
	mux.HandleFunc("/api/mode", s.apiMode)
	mux.HandleFunc("/api/toggle_prevent_same_game", s.apiTogglePreventSameGame)
	mux.HandleFunc("/files/", s.handleFiles)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/files/list.json", s.handleFilesList)
	mux.HandleFunc("/api/BizhawkFiles.zip", s.handleBizhawkFilesZip)
	// Plugin file serving
	mux.HandleFunc("/files/plugins/", s.handlePluginFiles)
	mux.HandleFunc("/state.json", s.handleStateJSON)
	mux.HandleFunc("/api/share_urls", s.apiShareURLs)
	mux.HandleFunc("/api/games", s.apiGames)
	mux.HandleFunc("/api/interval", s.apiInterval)
	mux.HandleFunc("/api/swap_player", s.apiSwapPlayer)
	mux.HandleFunc("/api/remove_player", s.apiRemovePlayer)
	mux.HandleFunc("/api/add_player", s.apiAddPlayer)
	mux.HandleFunc("/api/swap_all_to_game", s.apiSwapAllToGame)
	// Completed games/instances routes
	mux.HandleFunc("/api/players/remove_all_completions", s.apiRemoveAllCompletions)
	mux.HandleFunc("/api/players/", s.handlePlayerCompletedRoutes)
	mux.HandleFunc("/api/games/", s.handleGameCompletedRoutes)
	mux.HandleFunc("/api/instances/", s.handleInstanceCompletedRoutes)
	// Plugin management routes
	mux.HandleFunc("/api/plugins", s.handlePluginsList)
	// Plugin management routes - handles settings and other plugin actions
	mux.HandleFunc("/api/plugins/", s.handlePluginAction)
	mux.HandleFunc("/api/open_roms_folder", s.handleOpenRomsFolder)
	mux.HandleFunc("/api/open_plugins_folder", s.handleOpenPluginsFolder)
	mux.HandleFunc("/api/message_player", s.apiMessagePlayer)
	mux.HandleFunc("/api/message_all", s.apiMessageAll)
	mux.HandleFunc("/api/fullscreen_toggle", s.apiFullscreenToggle)
	// Config management endpoints
	mux.HandleFunc("/api/check_player_config", s.apiCheckPlayerConfig)
	mux.HandleFunc("/api/update_player_config", s.apiUpdatePlayerConfig)
	mux.HandleFunc("/api/set_config_keys", s.apiSetConfigKeys)
	// Save state management endpoints
	mux.HandleFunc("/save/upload", s.handleSaveUpload)
	mux.HandleFunc("/save/no-save", s.handleNoSaveState)
	mux.HandleFunc("/save/", s.handleSaveDownload)
}

// SetOpenInFileManager overrides launching the system file manager (e.g. Explorer).
// Pass nil to restore the default. Integration tests should use a no-op.
func (s *Server) SetOpenInFileManager(fn func(path string) error) {
	s.openInFileManager = fn
}

func (s *Server) SetHost(host string) {
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Host = host
	})
}

func (s *Server) PersistedHost() string { return s.SnapshotState().Host }

func (s *Server) SetPort(port int) {
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		st.Port = port
	})
}

func (s *Server) PersistedPort() int { return s.SnapshotState().Port }

func (s *Server) StartBroadcaster(ctx context.Context) error {
	if b := s.broadcaster.Load(); b != nil {
		return b.Start(ctx)
	}
	st := s.SnapshotState()
	host := st.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := st.Port
	if port == 0 {
		port = 8080
	}
	nb := NewDiscoveryBroadcaster(protocol.GetDefaultDiscoveryConfig(), host, port, s.GetServerName())
	if !s.broadcaster.CompareAndSwap(nil, nb) {
		return s.broadcaster.Load().Start(ctx)
	}
	return nb.Start(ctx)
}

// StopBroadcaster stops the discovery broadcaster without taking s.mu.
func (s *Server) StopBroadcaster() error {
	b := s.broadcaster.Swap(nil)
	if b == nil {
		return nil
	}
	return b.Stop()
}

// GetServerName returns a human-readable name for this server
func (s *Server) GetServerName() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "BizShuffle"
	}
	return fmt.Sprintf("%s Server", hostname)
}

// withLock executes fn while holding the write lock. This centralizes
// locking to avoid manual mu.Lock()/mu.Unlock() footguns in callers.
func (s *Server) withLock(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

// withRLock executes fn while holding the read lock.
func (s *Server) withRLock(fn func()) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn()
}

func (s *Server) currentPlayer(player string) protocol.Player {
	playerInfo := s.GetGameForPlayer(player)
	if playerInfo.Game != "" {
		return playerInfo
	}
	handler := s.GetGameModeHandler()
	playerInfo = handler.GetPlayer(player)
	return playerInfo
}
