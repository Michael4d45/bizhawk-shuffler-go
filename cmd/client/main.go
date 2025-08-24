package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yourname/bizshuffle/internal"
	"github.com/yourname/bizshuffle/internal/types"
)

func main() {
	var serverURL string
	var verbose bool
	// Accept either websocket (ws:// or wss://) or http(s) base URL so
	// the same flag can be used for websocket connections and HTTP API
	// downloads (e.g. http://host:port or ws://host:port/ws).
	flag.StringVar(&serverURL, "server", "", "server URL (ws://, wss://, http:// or https://) e.g. ws://host:port/ws or http://host:port")
	flag.BoolVar(&verbose, "v", false, "enable verbose logging to stdout and file")
	flag.Parse()

	// initialize logging
	logFile, err := initLogging(verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logging: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logFile.Close() }()

	cfgFile := filepath.Join(".", "client_config.json")
	var cfg map[string]string
	if b, err := os.ReadFile(cfgFile); err == nil {
		json.Unmarshal(b, &cfg)
		log.Printf("loaded config from %s", cfgFile)
	} else {
		cfg = map[string]string{}
	}

	// If an existing config provides a websocket URL, normalize it to an
	// HTTP base (no path) so code that appends API paths (e.g. /api/...) will
	// work. We modify the in-memory cfg only; writing back to disk happens
	// later when persisting defaults or changes.
	if s, ok := cfg["server"]; ok && s != "" {
		if u, err := url.Parse(s); err == nil {
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
	}

	reader := bufio.NewReader(os.Stdin)

	// Prompt for serverURL until a valid value is provided. Accept ws/wss or http/https.
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
		// Normalize and store the HTTP base in cfg as the authoritative server address
		// so other code can append paths like /api/BizhawkFiles.zip or /save/... reliably.
		u, err := url.Parse(serverURL)
		if err != nil {
			fmt.Printf("invalid server URL: %v\n", err)
			serverURL = ""
			continue
		}
		// If the user supplied ws/wss, convert scheme to http/https for cfg storage
		switch u.Scheme {
		case "ws":
			u.Scheme = "http"
		case "wss":
			u.Scheme = "https"
		}
		// Clear any path, query, or fragment for cfg storage (we want base URL)
		u.Path = ""
		u.RawQuery = ""
		u.Fragment = ""
		cfg["server"] = u.String()
		// Keep serverURL as originally provided so websocket code can use it later
	}

	// Prompt for player name until non-empty
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
		// persist canonical key "name" only; other code should fall back to this
		cfg["name"] = name
		break
	}

	// Persist initial config (name/server) if they were just created.
	if jb, _ := json.MarshalIndent(cfg, "", "  "); jb != nil {
		_ = os.WriteFile(cfgFile, jb, 0644)
		log.Printf("wrote initial config to %s", cfgFile)
	}

	// Apply sensible defaults when missing (so auto-install can work)
	if cfg["bizhawk_download_url"] == "" {
		switch goos := runtime.GOOS; goos {
		case "windows":
			cfg["bizhawk_download_url"] = "https://github.com/TASEmulators/BizHawk/releases/download/2.10/BizHawk-2.10-win-x64.zip"
		case "linux":
			cfg["bizhawk_download_url"] = "https://github.com/TASEmulators/BizHawk/releases/download/2.10/BizHawk-2.10-linux-x64.tar.gz"
		default:
			err := fmt.Errorf("no default BizHawk download URL for OS: %s", goos)
			log.Printf("%v", err)
			os.Exit(1)
		}
	}
	if cfg["bizhawk_path"] == "" {
		cfg["bizhawk_path"] = filepath.Join("BizHawk-2.10-win-x64", "EmuHawk.exe")
	}
	if cfg["rom_dir"] == "" {
		cfg["rom_dir"] = "roms"
	}
	if cfg["save_dir"] == "" {
		cfg["save_dir"] = "saves"
	}
	if cfg["bizhawk_ipc_port"] == "" {
		cfg["bizhawk_ipc_port"] = "55355"
	}
	// persist defaults
	if jb2, _ := json.MarshalIndent(cfg, "", "  "); jb2 != nil {
		_ = os.WriteFile(cfgFile, jb2, 0644)
		log.Printf("persisted default config to %s", cfgFile)
	}

	// HTTP client used for downloads/install
	httpClient := &http.Client{Timeout: 0}

	// Attempt to ensure BizHawk is installed (fatal if missing)
	if err := EnsureBizHawkInstalled(httpClient, cfg); err != nil {
		log.Fatalf("EnsureBizHawkInstalled: %v", err)
	}
	// After potential install, persist any updates to bizhawk_path
	if jb, _ := json.MarshalIndent(cfg, "", "  "); jb != nil {
		_ = os.WriteFile(cfgFile, jb, 0644)
		log.Printf("persisted config after BizHawk install: %s", cfgFile)
	}

	// Build websocket URL (wsURL) and an HTTP base URL (serverHTTP).
	// Prefer using cfg["server"] (normalized to http/https base) for HTTP operations
	// so callers can append API paths reliably. For websocket, use the original
	// serverURL if it was provided as ws:// or wss://; otherwise derive from cfg.
	var wsURL string
	// Determine serverHTTP base (from cfg if present)
	serverHTTP := ""
	if s, ok := cfg["server"]; ok && s != "" {
		serverHTTP = s
	}
	// If user passed a ws/wss directly on the flag, use it for websocket.
	if strings.HasPrefix(serverURL, "ws://") || strings.HasPrefix(serverURL, "wss://") {
		u, err := url.Parse(serverURL)
		if err != nil {
			log.Fatalf("invalid server url %q: %v", serverURL, err)
		}
		if u.Path == "" || u.Path == "/" {
			u.Path = "/ws"
		} else if !strings.HasSuffix(u.Path, "/ws") {
			u.Path = strings.TrimRight(u.Path, "/") + "/ws"
		}
		wsURL = u.String()
		// Ensure serverHTTP is populated (convert ws->http) if it wasn't already
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
		// serverURL wasn't a websocket; derive wsURL from serverHTTP (cfg)
		if serverHTTP == "" {
			log.Fatalf("no server configured for websocket and -server flag not provided")
		}
		hu, err := url.Parse(serverHTTP)
		if err != nil {
			log.Fatalf("invalid configured server %q: %v", serverHTTP, err)
		}
		switch hu.Scheme {
		case "http":
			hu.Scheme = "ws"
		case "https":
			hu.Scheme = "wss"
		}
		// ensure websocket path is present
		if hu.Path == "" || hu.Path == "/" {
			hu.Path = "/ws"
		} else if !strings.HasSuffix(hu.Path, "/ws") {
			hu.Path = strings.TrimRight(hu.Path, "/") + "/ws"
		}
		wsURL = hu.String()
	}

	// create a cancellable context before attempting network actions so
	// that retries can be aborted via signals or other cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// single persistent signal channel. Actual handling is performed later
	// (after websocket variables are declared) so we can safely close ws.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	// non-blocking websocket connect: create a send queue and a connected
	// notification channel. This lets BizHawk start while the client retries
	// connecting to the server in the background.
	sendQueue := make(chan types.Command, 64)
	connectedCh := make(chan struct{})
	var ws *websocket.Conn
	var wsMu sync.Mutex

	// writeJSON enqueues outgoing commands; they'll be sent once the
	// background connector establishes a websocket.
	writeJSON := func(cmd types.Command) error {
		select {
		case sendQueue <- cmd:
			return nil
		case <-time.After(2 * time.Second):
			return fmt.Errorf("send queue full or no connection")
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// background connector: retry until we connect or context is canceled.
	go func() {
		dialer := websocket.Dialer{}
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			conn, _, err := dialer.Dial(wsURL, nil)
			if err != nil {
				log.Printf("dial: %v; retrying in 2s", err)
				select {
				case <-time.After(2 * time.Second):
					continue
				case <-ctx.Done():
					return
				}
			}
			wsMu.Lock()
			ws = conn
			wsMu.Unlock()
			log.Printf("websocket connected to server")
			// start sender goroutine
			go func() {
				for cmd := range sendQueue {
					wsMu.Lock()
					if ws == nil {
						wsMu.Unlock()
						log.Printf("ws not available when sending command")
						continue
					}
					if err := ws.WriteJSON(cmd); err != nil {
						log.Printf("ws write error: %v", err)
					}
					wsMu.Unlock()
				}
			}()
			close(connectedCh)
			return
		}
	}()

	// The writeJSON function above enqueues commands into sendQueue. A background
	// sender (started by the connector) will actually write them once the
	// websocket is established. This avoids blocking BizHawk startup.

	// ensure sendQueue and ws are cleaned up when context is cancelled
	go func() {
		<-ctx.Done()
		wsMu.Lock()
		if ws != nil {
			_ = ws.Close()
			ws = nil
		}
		wsMu.Unlock()
		// closing sendQueue signals the sender goroutine to exit
		close(sendQueue)
	}()

	// send hello
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

	// track whether we've observed a HELLO from Lua so we can include it in NACK diagnostics
	var ipcReadyMu sync.Mutex
	ipcReady := false

	// serverHTTP is already determined above (from cfg or derived from ws URL)

	// helper: fetch server state (/state.json) and return running bool and player's current game
	fetchServerState := func() (running bool, playerGame string) {
		running = true
		playerGame = ""
		// Build state URL by appending /state.json to serverHTTP
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
				// also support legacy key "current" if present
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
			// special sentinel to indicate IPC disconnected
			if line == internal.MsgDisconnected || line == "__BIZHAWK_IPC_DISCONNECTED__" {
				log.Printf("bizhawk ipc: disconnected detected from readLoop")
				// cancel main context so program can exit cleanly
				cancel()
				break
			}
			log.Printf("lua: %s", line)
			if strings.HasPrefix(line, "HELLO") {
				log.Printf("received HELLO from lua, sending sync")
				ipcReadyMu.Lock()
				ipcReady = true
				ipcReadyMu.Unlock()

				// fetch server state and player's current game so SYNC includes correct info
				running, playerGame := fetchServerState()
				if playerGame == "" {
					// if server didn't provide a current game for this player, fall back to empty
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
		// ensure we cancel the main context if not already canceled
		cancel()
	}()

	dl := internal.NewDownloader(serverHTTP, "./roms")

	// helper: upload save file to server
	uploadSave := func(localPath, player, game string) error {
		log.Printf("uploadSave: %s (player=%s game=%s)", localPath, player, game)
		// notify server we're uploading
		_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "uploading"}})
		f, err := os.Open(localPath)
		if err != nil {
			_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "idle"}})
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
		// include filename explicitly so server can use it atomically
		_ = w.WriteField("filename", filepath.Base(localPath))
		w.Close()
		req, err := http.NewRequestWithContext(context.Background(), "POST", serverHTTP+"/save/upload", &buf)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", w.FormDataContentType())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "idle"}})
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			data, _ := io.ReadAll(resp.Body)
			log.Printf("uploadSave failed status=%s body=%s", resp.Status, string(data))
			_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "idle"}})
			return fmt.Errorf("upload failed: %s %s", resp.Status, string(data))
		}
		// done uploading
		_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "idle"}})
		return nil
	}

	// helper: download save from server into ./saves/<player>/<file>
	downloadSave := func(ctx context.Context, player, filename string) error {
		log.Printf("downloadSave: player=%s file=%s", player, filename)
		// notify server we're downloading
		_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "downloading"}})
		destDir := filepath.Join("./saves", player)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "idle"}})
			return err
		}
		base := strings.TrimSuffix(serverHTTP, "/")
		// escape path parts
		p := "/save/" + url.PathEscape(player) + "/" + url.PathEscape(filename)
		fetch := base + p
		req, _ := http.NewRequestWithContext(ctx, "GET", fetch, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("downloadSave bad status: %s body=%s", resp.Status, string(body))
			if resp.StatusCode == http.StatusNotFound {
				return ErrNotFound
			}
			return fmt.Errorf("bad status: %s %s", resp.Status, string(body))
		}
		outPath := filepath.Join(destDir, filename)
		out, err := os.Create(outPath)
		if err != nil {
			_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "idle"}})
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		_ = writeJSON(types.Command{Cmd: types.CmdStatus, Payload: map[string]string{"status": "idle"}})
		return err
	}

	var bhCmd *exec.Cmd
	var bhMu sync.Mutex
	// Launch BizHawk via helper which will attempt install if necessary.
	bh, err := LaunchBizHawk(ctx, cfg, httpClient)
	if err != nil {
		log.Fatalf("failed to launch BizHawk: %v", err)
	} else {
		bhCmd = bh
		// Persist any cfg updates (e.g., bizhawk_path)
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
				// ensure shutdown
				cancel()
				// safe close of websocket
				wsMu.Lock()
				if ws != nil {
					_ = ws.Close()
					ws = nil
				}
				wsMu.Unlock()
			}()
		}
	}

	// single goroutine that waits for signals and attempts graceful shutdown
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
			// ensure shutdown of main context
			cancel()
			// safe close of websocket under mutex
			wsMu.Lock()
			if ws != nil {
				_ = ws.Close()
				ws = nil
			}
			wsMu.Unlock()
		}
	}()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		// wait until background connector signals connected (or ctx cancelled)
		select {
		case <-connectedCh:
		case <-ctx.Done():
			return
		}
		for {
			var cmd types.Command
			wsMu.Lock()
			conn := ws
			wsMu.Unlock()
			if conn == nil {
				// connection lost; exit so main can cancel/restart as needed
				log.Printf("ws connection lost before read")
				return
			}
			if err := conn.ReadJSON(&cmd); err != nil {
				log.Printf("ws read: %v", err)
				return
			}
			log.Printf("server->client cmd: %s", cmd.Cmd)

			sendAck := func(id string) {
				_ = writeJSON(types.Command{Cmd: types.CmdAck, ID: id})
			}
			sendNack := func(id, reason string) {
				_ = writeJSON(types.Command{Cmd: types.CmdNack, ID: id, Payload: map[string]string{"reason": reason}})
			}

			sendNackDetailed := func(id, shortReason, filePath string, errDetails error) {
				// collect file diagnostics if a path is provided
				diag := map[string]any{"reason": shortReason}
				if filePath != "" {
					if fi, err := os.Stat(filePath); err == nil {
						diag["file_exists"] = true
						diag["file_size"] = fi.Size()
						diag["file_modtime"] = fi.ModTime().Unix()
					} else {
						diag["file_exists"] = false
						diag["file_stat_error"] = err.Error()
					}
				}
				ipcReadyMu.Lock()
				diag["ipc_ready"] = ipcReady
				ipcReadyMu.Unlock()
				if errDetails != nil {
					diag["error_detail"] = errDetails.Error()
				}
				// send as JSON payload (server may be expecting map[string]string but will accept map[string]any)
				_ = writeJSON(types.Command{Cmd: types.CmdNack, ID: id, Payload: diag})
			}

			switch cmd.Cmd {
			case types.CmdStart:
				go func(id string) {
					game := ""
					if m, ok := cmd.Payload.(map[string]any); ok {
						if g, ok := m["game"].(string); ok {
							game = g
						}
					}
					if game != "" {
						ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
						if err := dl.EnsureFile(ctx2, game); err != nil {
							cancel2()
							sendNack(id, "download failed: "+err.Error())
							return
						}
						cancel2()
					}
					log.Printf("handling start command for game=%s", game)
					if err := bipc.SendStart(ctx, time.Now().Unix(), game); err != nil {
						sendNack(id, err.Error())
						return
					}
					sendAck(id)
				}(cmd.ID)
			case types.CmdPause:
				go func(id string) {
					log.Printf("handling pause command")
					if err := bipc.SendPause(ctx, nil); err != nil {
						sendNack(id, err.Error())
						return
					}
					sendAck(id)
				}(cmd.ID)
			case types.CmdResume:
				go func(id string) {
					log.Printf("handling resume command")
					if err := bipc.SendResume(ctx, nil); err != nil {
						sendNack(id, err.Error())
						return
					}
					sendAck(id)
				}(cmd.ID)
			case types.CmdSwap:
				go func(id string) {
					game := ""
					if m, ok := cmd.Payload.(map[string]any); ok {
						if g, ok := m["game"].(string); ok {
							game = g
						}
					}
					if game == "" {
						sendNack(id, "missing game")
						return
					}
					// 1) ask Lua to save current state to a local file
					player := cfg["name"]
					saveDir := filepath.Join("./saves", player)
					if err := os.MkdirAll(saveDir, 0755); err != nil {
						sendNack(id, "mkdir failed: "+err.Error())
						return
					}
					localSave := filepath.Join(saveDir, game+".state")
					log.Printf("requesting save to localSave=%s", localSave)
					if err := bipc.SendSave(ctx, localSave); err != nil {
						log.Printf("SendSave failed: %v", err)
						// If the IPC isn't ready, we want to surface that but not treat
						// a missing save as a hard failure here — continue so game can load fresh.
						sendNackDetailed(id, "save failed", localSave, err)
						// continue rather than return: attempt upload will likely fail but
						// we prefer to proceed so the game can still be started without a save
						// if the save file does not exist.
						// (Don't return)
					}
					// 2) upload saved file to server
					if err := uploadSave(localSave, player, game); err != nil {
						log.Printf("uploadSave failed: %v", err)
						// If the file isn't present locally, upload will fail; log and continue.
						if os.IsNotExist(err) {
							log.Printf("local save missing, continuing without upload: %s", localSave)
							// continue without returning/nacking
						} else {
							sendNackDetailed(id, "upload failed", localSave, err)
							return
						}
					}
					// 3) ensure ROM present locally
					ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
					log.Printf("ensuring ROM present for game=%s", game)
					if err := dl.EnsureFile(ctx2, game); err != nil {
						cancel2()
						sendNack(id, "download failed: "+err.Error())
						return
					}
					cancel2()
					// 4) instruct Lua to swap to the new game
					log.Printf("sending swap to lua for game=%s", game)
					if err := bipc.SendSwap(ctx, time.Now().Unix(), game); err != nil {
						sendNack(id, err.Error())
						return
					}
					sendAck(id)
				}(cmd.ID)

			case types.CmdDownloadSave:
				go func(id string) {
					player := ""
					file := ""
					if m, ok := cmd.Payload.(map[string]any); ok {
						if p, ok := m["player"].(string); ok {
							player = p
						}
						if f, ok := m["file"].(string); ok {
							file = f
						}
					}
					if player == "" || file == "" {
						sendNack(id, "missing player or file")
						return
					}
					ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel2()
					log.Printf("handling download_save for player=%s file=%s", player, file)
					if err := downloadSave(ctx2, player, file); err != nil {
						if errors.Is(err, ErrNotFound) {
							// server reports the save doesn't exist. That's ok; ack and do not LOAD.
							log.Printf("download_save: remote save not found for player=%s file=%s; acking without LOAD", player, file)
							sendAck(id)
							return
						}
						sendNack(id, "download save failed: "+err.Error())
						return
					}
					// instruct Lua to load the downloaded save
					localPath := filepath.Join("./saves", player, file)
					if err := bipc.SendCommand(ctx2, "LOAD", localPath); err != nil {
						sendNack(id, "load failed: "+err.Error())
						return
					}
					sendAck(id)
				}(cmd.ID)
			case types.CmdClearSaves:
				go func(id string) {
					if err := bipc.SendMessage(ctx, "clear_saves"); err != nil {
						sendNack(id, err.Error())
						return
					}
					sendAck(id)
				}(cmd.ID)
			case types.CmdReset:
				go func(id string) {
					if err := bipc.SendMessage(ctx, "reset"); err != nil {
						sendNack(id, err.Error())
						return
					}
					sendAck(id)
				}(cmd.ID)

			case types.CmdGamesUpdate:
				// server sends {games: []string, main_games: []GameEntry}
				// Only ensure active games listed in payload.games are present locally.
				go func(payload any) {
					required := make(map[string]struct{})
					active := make(map[string]struct{})
					if m, ok := payload.(map[string]any); ok {
						// only process the active games list
						if gs, ok := m["games"].([]any); ok {
							for _, gi := range gs {
								if sname, ok := gi.(string); ok {
									required[sname] = struct{}{}
									active[sname] = struct{}{}
								}
							}
						}
						// include extra_files for any main_games whose primary file is active
						if mg, ok := m["main_games"].([]any); ok {
							for _, mei := range mg {
								if em, ok := mei.(map[string]any); ok {
									if f, ok := em["file"].(string); ok {
										if _, isActive := active[f]; isActive {
											if extras, ok := em["extra_files"].([]any); ok {
												for _, ex := range extras {
													if exs, ok := ex.(string); ok {
														required[exs] = struct{}{}
													}
												}
											}
										}
									}
								}
							}
						}
					}
					var wg sync.WaitGroup
					errCh := make(chan error, 8)
					for name := range required {
						n := name
						wg.Add(1)
						go func(fname string) {
							defer wg.Done()
							ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
							defer cancel2()
							if err := dl.EnsureFile(ctx2, fname); err != nil {
								errCh <- fmt.Errorf("failed to download %s: %w", fname, err)
								return
							}
							log.Printf("games_update: ensured file %s", fname)
						}(n)
					}
					wg.Wait()
					close(errCh)
					// collect errors and report back to server
					errList := []string{}
					for e := range errCh {
						log.Printf("games_update error: %v", e)
						errList = append(errList, e.Error())
					}
					hasFiles := len(errList) == 0
					// report back to server
					ackPayload := map[string]any{"has_files": hasFiles}
					if !hasFiles {
						ackPayload["errors"] = errList
					}
					_ = writeJSON(types.Command{Cmd: types.CmdGamesUpdateAck, ID: fmt.Sprintf("%d", time.Now().UnixNano()), Payload: ackPayload})
				}(cmd.Payload)
			default:
				sendAck(cmd.ID)
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
}
