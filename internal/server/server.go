package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michael4d45/bizshuffle/internal/types"
)

// ErrTimeout is exported so callers can detect timeout waiting for a client ack/nack.
var ErrTimeout = fmt.Errorf("timeout waiting for result")

// Server encapsulates all state and connected websocket clients.
type Server struct {
	mu          sync.Mutex
	state       types.ServerState
	conns       map[*websocket.Conn]*wsClient
	players     map[string]*wsClient
	upgrader    websocket.Upgrader
	pending     map[string]chan string
	schedulerCh chan struct{}
}

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

	// Save state management endpoints
	mux.HandleFunc("/save/upload", s.handleSaveUpload)
	mux.HandleFunc("/save/", s.handleSaveDownload)
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
	defer s.mu.Unlock()
	if s.state.Port == port {
		return
	}
	s.state.Port = port
	s.state.UpdatedAt = time.Now()
	st := s.state
	if err := s.saveState(); err != nil {
		log.Printf("failed to persist port: %v", err)
	} else {
		log.Printf("port updated to %d", st.Port)
	}
}

// PersistedPort returns the persisted port if present.
func (s *Server) PersistedPort() int { s.mu.Lock(); defer s.mu.Unlock(); return s.state.Port }

func (s *Server) currentPlayer(player string) types.Player {
	playerInfo := s.GetGameForPlayer(player)
	if playerInfo.Game != "" {
		return playerInfo
	}
	handler := s.GetGameModeHandler()
	playerInfo = handler.GetPlayer(player)
	return playerInfo
}
