package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michael4d45/bizshuffle/internal/types"
)

// Server encapsulates all state and connected websocket clients.
type Server struct {
	mu                   sync.RWMutex
	pendingInstancecount int
	state                types.ServerState
	conns                map[*websocket.Conn]*wsClient
	playerClients        map[string]*wsClient
	adminClients         map[string]*wsClient
	upgrader             websocket.Upgrader
	pending              map[string]chan string
	schedulerCh          chan struct{}
	broadcaster          *DiscoveryBroadcaster
	saveChan             chan struct{}
	saveTimer            *time.Timer
	saveMutex            sync.Mutex
}

// ErrTimeout is exported so callers can detect timeout waiting for a client ack/nack.
var ErrTimeout = fmt.Errorf("timeout waiting for result")

// New creates and initializes a Server, loading state and starting the scheduler.
func New() *Server {
	s := &Server{
		state: types.ServerState{
			Running:             false,
			SwapEnabled:         true,
			Mode:                types.GameModeSync,
			MainGames:           []types.GameEntry{},
			Plugins:             make(map[string]types.Plugin),
			GameSwapInstances:   []types.GameSwapInstance{},
			Games:               []string{},
			Players:             map[string]types.Player{},
			UpdatedAt:           time.Now(),
			MinIntervalSecs:     5,
			MaxIntervalSecs:     300,
			PreventSameGameSwap: false, // Default to false
		},
		conns:         make(map[*websocket.Conn]*wsClient),
		playerClients: make(map[string]*wsClient),
		adminClients:  make(map[string]*wsClient),
		upgrader:      websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		pending:       make(map[string]chan string),
		schedulerCh:   make(chan struct{}, 1),
		saveChan:      make(chan struct{}, 1),
	}
	s.loadState()
	_ = os.MkdirAll("./files", 0755)
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
	mux.HandleFunc("/api/do_swap", s.apiDoSwap)
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
	mux.HandleFunc("/save/no-save", s.handleNoSaveState)
	mux.HandleFunc("/save/", s.handleSaveDownload)
}

func (s *Server) SetHost(host string) {
	s.UpdateStateAndPersist(func(st *types.ServerState) {
		st.Host = host
	})
}

func (s *Server) PersistedHost() string { return s.SnapshotState().Host }

func (s *Server) SetPort(port int) {
	s.UpdateStateAndPersist(func(st *types.ServerState) {
		st.Port = port
	})
}

func (s *Server) PersistedPort() int { return s.SnapshotState().Port }

func (s *Server) StartBroadcaster(ctx context.Context) error {
	var startedErr error
	s.withLock(func() {
		if s.broadcaster != nil {
			startedErr = s.broadcaster.Start(ctx)
			return
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
	})
	if startedErr != nil {
		return startedErr
	}
	return s.broadcaster.Start(ctx)
}

// StopBroadcaster stops the discovery broadcaster
func (s *Server) StopBroadcaster() error {
	var stopErr error
	s.withLock(func() {
		if s.broadcaster != nil {
			stopErr = s.broadcaster.Stop()
		}
	})
	return stopErr
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

func (s *Server) currentPlayer(player string) types.Player {
	playerInfo := s.GetGameForPlayer(player)
	if playerInfo.Game != "" {
		return playerInfo
	}
	handler := s.GetGameModeHandler()
	playerInfo = handler.GetPlayer(player)
	return playerInfo
}
