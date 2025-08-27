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
			Running:     false,
			SwapEnabled: true,
			Mode:        types.GameModeSync,
			Games:       []string{},
			MainGames:   []types.GameEntry{},
			Players:     map[string]types.Player{},
			UpdatedAt:   time.Now(),
		},
		conns:       make(map[*websocket.Conn]*wsClient),
		players:     make(map[string]*wsClient),
		upgrader:    websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		pending:     make(map[string]chan string),
		schedulerCh: make(chan struct{}, 1),
	}
	s.loadState()
	_ = os.MkdirAll("./files", 0755)
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

// SaveState persists state to disk.
func (s *Server) SaveState() error { return s.saveState() }

// State returns a copy of the current ServerState.
func (s *Server) State() types.ServerState { s.mu.Lock(); defer s.mu.Unlock(); return s.state }

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

// currentGameForPlayer determines what game a player should be playing now.
// Order of determination:
// 1) If the player's Current field is set in persisted state, return that.
// 2) Delegate to the current game mode handler for mode-specific logic.
// The function does not modify state; callers may persist any changes if desired.
func (s *Server) currentGameForPlayer(player string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 1) persisted per-player current
	if p, ok := s.state.Players[player]; ok {
		if p.Current != "" {
			return p.Current
		}
	}
	// 2) delegate to game mode handler
	handler, err := getGameModeHandler(s.state.Mode)
	if err != nil {
		// Fallback to sync mode behavior if unknown mode
		handler = &SyncModeHandler{}
	}
	return handler.GetCurrentGameForPlayer(s, player)
}
