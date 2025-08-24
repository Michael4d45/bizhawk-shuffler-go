package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/michael4d45/bizshuffle/internal"
	"github.com/michael4d45/bizshuffle/internal/types"
)

// ErrNotFound is returned when a requested remote save/file is not present on the server
var ErrNotFound = errors.New("not found")

// Run is the main entrypoint for the client logic. It mirrors the previous
// cmd/client/main.go contents but is refactored into an internal package so
// the command's main remains tiny.
func Run(args []string) error {
	var serverURL string
	var verbose bool
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	fs.StringVar(&serverURL, "server", "", "server URL (ws://, wss://, http:// or https://)")
	fs.BoolVar(&verbose, "v", false, "enable verbose logging to stdout and file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	logFile, err := InitLogging(verbose)
	if err != nil {
		return fmt.Errorf("failed to init logging: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	cfgFile := filepath.Join(".", "client_config.json")
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if len(cfg) > 0 {
		log.Printf("loaded config from %s", cfgFile)
	}

	// normalize stored server url
	cfg.NormalizeServer()

	reader := bufio.NewReader(os.Stdin)

	for serverURL == "" {
		if s, ok := cfg["server"]; ok && s != "" {
			serverURL = s
			break
		}
		fmt.Print("Server URL (ws://host:port/ws or http://host:port): ")
		line, _ := reader.ReadString('\n')
		serverURL = strings.TrimSpace(line)
		if serverURL == "" {
			fmt.Println("server URL cannot be empty")
			continue
		}
		if !(strings.HasPrefix(serverURL, "ws://") || strings.HasPrefix(serverURL, "wss://") || strings.HasPrefix(serverURL, "http://") || strings.HasPrefix(serverURL, "https://")) {
			fmt.Println("server URL must start with ws://, wss://, http:// or https://")
			serverURL = ""
			continue
		}
		u, err := url.Parse(serverURL)
		if err != nil {
			fmt.Printf("invalid server URL: %v\n", err)
			serverURL = ""
			continue
		}
		switch u.Scheme {
		case "ws":
			u.Scheme = "http"
		case "wss":
			u.Scheme = "https"
		}
		u.Path = ""
		u.RawQuery = ""
		u.Fragment = ""
		cfg["server"] = u.String()
	}

	for {
		if n, ok := cfg["name"]; ok && strings.TrimSpace(n) != "" {
			break
		}
		fmt.Print("Player name: ")
		line, _ := reader.ReadString('\n')
		name := strings.TrimSpace(line)
		if name == "" {
			fmt.Println("player name cannot be empty")
			continue
		}
		cfg["name"] = name
		break
	}

	if jb, _ := json.MarshalIndent(cfg, "", "  "); jb != nil {
		_ = os.WriteFile(cfgFile, jb, 0644)
		log.Printf("wrote initial config to %s", cfgFile)
	}

	if err := cfg.EnsureDefaults(); err != nil {
		return fmt.Errorf("EnsureDefaults: %w", err)
	}
	if err := cfg.Save(cfgFile); err == nil {
		log.Printf("persisted default config to %s", cfgFile)
	}

	httpClient := &http.Client{Timeout: 0}
	// EnsureBizHawkInstalled and LaunchBizHawk currently accept map[string]string
	// so convert Config to that shape for now.
	cfgMap := map[string]string{}
	for k, v := range cfg {
		cfgMap[k] = v
	}
	if err := EnsureBizHawkInstalled(httpClient, cfgMap); err != nil {
		return fmt.Errorf("EnsureBizHawkInstalled: %w", err)
	}
	// persist any changes EnsureBizHawkInstalled may have made
	for k, v := range cfgMap {
		cfg[k] = v
	}
	if err := cfg.Save(cfgFile); err == nil {
		log.Printf("persisted config after BizHawk install: %s", cfgFile)
	}

	var wsURL string
	serverHTTP := ""
	if s, ok := cfg["server"]; ok && s != "" {
		serverHTTP = s
	}
	if strings.HasPrefix(serverURL, "ws://") || strings.HasPrefix(serverURL, "wss://") {
		u, err := url.Parse(serverURL)
		if err != nil {
			return fmt.Errorf("invalid server url %q: %w", serverURL, err)
		}
		if u.Path == "" || u.Path == "/" {
			u.Path = "/ws"
		} else if !strings.HasSuffix(u.Path, "/ws") {
			u.Path = strings.TrimRight(u.Path, "/") + "/ws"
		}
		wsURL = u.String()
		if serverHTTP == "" {
			hu := *u
			switch hu.Scheme {
			case "ws":
				hu.Scheme = "http"
			case "wss":
				hu.Scheme = "https"
			}
			hu.Path = ""
			hu.RawQuery = ""
			hu.Fragment = ""
			serverHTTP = hu.String()
		}
	} else {
		if serverHTTP == "" {
			return fmt.Errorf("no server configured for websocket and -server flag not provided")
		}
		hu, err := url.Parse(serverHTTP)
		if err != nil {
			return fmt.Errorf("invalid configured server %q: %w", serverHTTP, err)
		}
		switch hu.Scheme {
		case "http":
			hu.Scheme = "ws"
		case "https":
			hu.Scheme = "wss"
		}
		if hu.Path == "" || hu.Path == "/" {
			hu.Path = "/ws"
		} else if !strings.HasSuffix(hu.Path, "/ws") {
			hu.Path = strings.TrimRight(hu.Path, "/") + "/ws"
		}
		wsURL = hu.String()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	// websocket client
	wsClient := NewWSClient(wsURL)
	wsClient.Start(ctx)
	defer wsClient.Stop()

	writeJSON := func(cmd types.Command) error {
		// keep the small timeout behavior
		done := make(chan error, 1)
		go func() {
			done <- wsClient.Send(cmd)
		}()
		select {
		case err := <-done:
			return err
		case <-time.After(2 * time.Second):
			return fmt.Errorf("send queue full or no connection")
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// incoming commands channel (buffered to avoid blocking the WS reader)
	cmdCh := make(chan types.Command, 64)
	wsClient.RegisterHandler(func(cmd types.Command) {
		select {
		case cmdCh <- cmd:
		default:
			log.Printf("incoming command dropped: %v", cmd.Cmd)
		}
	})

	hello := types.Command{Cmd: types.CmdHello, Payload: map[string]string{"name": cfg["name"]}, ID: fmt.Sprintf("%d", time.Now().UnixNano())}
	_ = writeJSON(hello)

	bipc := internal.NewBizhawkIPC("127.0.0.1", 55355)
	if err := bipc.Start(ctx); err != nil {
		log.Printf("bizhawk ipc start: %v", err)
	} else {
		log.Printf("bizhawk ipc started")
	}
	defer func() {
		log.Printf("closing bizhawk ipc")
		bipc.Close()
	}()

	var ipcReadyMu sync.Mutex
	ipcReady := false

	fetchServerState := func() (running bool, playerGame string) {
		running = true
		playerGame = ""
		if serverHTTP == "" {
			return running, playerGame
		}
		resp, err := http.Get(strings.TrimRight(serverHTTP, "/") + "/state.json")
		if err != nil {
			log.Printf("failed to fetch /state.json: %v", err)
			return running, playerGame
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Printf("/state.json returned status: %s", resp.Status)
			return running, playerGame
		}
		var st struct {
			Running bool                      `json:"running"`
			Players map[string]map[string]any `json:"players"`
		}
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&st); err != nil {
			log.Printf("failed to decode /state.json: %v", err)
			return running, playerGame
		}
		running = st.Running
		if st.Players != nil {
			if p, ok := st.Players[cfg["name"]]; ok {
				if v, ok2 := p["current_game"]; ok2 {
					if s, ok4 := v.(string); ok4 {
						playerGame = s
					}
				}
				if playerGame == "" {
					if cur, ok2 := p["current"].(string); ok2 {
						playerGame = cur
					}
				}
			}
		}
		return running, playerGame
	}

	go func() {
		for line := range bipc.Incoming() {
			if line == internal.MsgDisconnected || line == "__BIZHAWK_IPC_DISCONNECTED__" {
				log.Printf("bizhawk ipc: disconnected detected from readLoop")
				cancel()
				break
			}
			log.Printf("lua: %s", line)
			if strings.HasPrefix(line, "HELLO") {
				log.Printf("received HELLO from lua, sending sync")
				ipcReadyMu.Lock()
				ipcReady = true
				ipcReadyMu.Unlock()
				running, playerGame := fetchServerState()
				if playerGame == "" {
					log.Printf("no current game for player from server state; sending empty game")
				}
				ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
				if err := bipc.SendSync(ctx2, playerGame, running, time.Now().Unix()); err != nil {
					log.Printf("SendSync failed: %v", err)
				}
				cancel2()
			}
		}
		log.Printf("bizhawk ipc incoming channel closed or handler exited")
		cancel()
	}()

	dl := internal.NewDownloader(serverHTTP, "./roms")

	// Use helper functions in saves.go for upload/download. writeJSON is used
	// here to emit status notifications like the previous implementation.
	uploadSave := func(localPath, player, game string) error {
		log.Printf("uploadSave: %s (player=%s game=%s)", localPath, player, game)
		_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "uploading"}})
		err := UploadSave(serverHTTP, localPath, player, game)
		if err != nil {
			_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "idle"}})
			return err
		}
		_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "idle"}})
		return nil
	}

	downloadSave := func(ctx context.Context, player, filename string) error {
		log.Printf("downloadSave: player=%s file=%s", player, filename)
		_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "downloading"}})
		err := DownloadSave(ctx, serverHTTP, player, filename)
		if err != nil {
			return err
		}
		_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "idle"}})
		return nil
	}

	var bhCmd *exec.Cmd
	var bhMu sync.Mutex
	bh, err := LaunchBizHawk(ctx, cfg, httpClient)
	if err != nil {
		return fmt.Errorf("failed to launch BizHawk: %w", err)
	} else {
		bhCmd = bh
		if jb, _ := json.MarshalIndent(cfg, "", "  "); jb != nil {
			_ = os.WriteFile(cfgFile, jb, 0644)
			log.Printf("persisted config after launching BizHawk")
		}
		if bhCmd != nil {
			log.Printf("monitoring BizHawk pid=%d", bhCmd.Process.Pid)
			go func() {
				err := bhCmd.Wait()
				if err != nil {
					log.Printf("BizHawk exited with error: %v", err)
				} else {
					log.Printf("BizHawk exited")
				}
				cancel()
			}()
		}
	}

	go func() {
		select {
		case <-ctx.Done():
			return
		case s := <-sigs:
			log.Printf("signal: %v", s)
			bhMu.Lock()
			if bhCmd != nil && bhCmd.Process != nil {
				log.Printf("sending SIGTERM to BizHawk pid=%d", bhCmd.Process.Pid)
				_ = bhCmd.Process.Signal(syscall.SIGTERM)
				time.AfterFunc(3*time.Second, func() {
					bhMu.Lock()
					defer bhMu.Unlock()
					if bhCmd != nil && bhCmd.ProcessState == nil {
						log.Printf("killing BizHawk pid=%d", bhCmd.Process.Pid)
						_ = bhCmd.Process.Kill()
					}
				})
			}
			bhMu.Unlock()
			cancel()
		}
	}()
	// construct controller with dependencies
	controller := NewController(cfg, bipc, dl, writeJSON, uploadSave, downloadSave, &ipcReadyMu, &ipcReady)

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			select {
			case <-ctx.Done():
				return
			case cmd := <-cmdCh:
				log.Printf("server->client cmd: %s", cmd.Cmd)
				controller.Handle(ctx, cmd)
			}
		}
	}()

	select {
	case <-ctx.Done():
		bhMu.Lock()
		if bhCmd != nil && bhCmd.Process != nil {
			bhCmd.Process.Kill()
		}
		bhMu.Unlock()
	case <-readDone:
		cancel()
	}
	return nil
}
