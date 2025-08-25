package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
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
	serverURL, cfg, cfgFile, logFile, httpClient, err := Bootstrap(args)
	if err != nil {
		return err
	}
	defer func() {
		if err := logFile.Close(); err != nil {
			_ = err
		}
	}()

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

	wsURL, serverHTTP, err := BuildWSAndHTTP(serverURL, cfg)
	if err != nil {
		return err
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
		return WriteJSONWithTimeout(ctx, wsClient, cmd, 2*time.Second)
	}

	// controller loop will register the ws handler and send the initial hello

	bipc := internal.NewBizhawkIPC("127.0.0.1", 55355)
	if err := bipc.Start(ctx); err != nil {
		log.Printf("bizhawk ipc start: %v", err)
	} else {
		log.Printf("bizhawk ipc started")
	}
	defer func() {
		log.Printf("closing bizhawk ipc")
		_ = bipc.Close()
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
		defer func() { _ = resp.Body.Close() }()
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

	StartIPCGoroutine(ctx, bipc, cfg["name"], fetchServerState, &ipcReadyMu, &ipcReady, cancel)

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
	bh, err := StartBizHawk(ctx, cfg, httpClient)
	if err != nil {
		return err
	}
	bhCmd = bh
	if jb, _ := json.MarshalIndent(cfg, "", "  "); jb != nil {
		_ = os.WriteFile(cfgFile, jb, 0644)
		log.Printf("persisted config after launching BizHawk")
	}
	if bhCmd != nil {
		log.Printf("monitoring BizHawk pid=%d", bhCmd.Process.Pid)
		MonitorProcess(bhCmd, func(err error) {
			_ = err // MonitorProcess already logs; propagate cancellation
			cancel()
		})
	}

	go func() {
		select {
		case <-ctx.Done():
			return
		case s := <-sigs:
			log.Printf("signal: %v", s)
			// TerminateProcess acquires the provided mutex internally. Do not
			// hold the mutex here or we'll deadlock when TerminateProcess
			// attempts to lock the same mutex (non-reentrant).
			log.Printf("terminating BizHawk due to signal")
			TerminateProcess(&bhCmd, &bhMu, 3*time.Second)
			cancel()
		}
	}()
	// construct controller and read loop
	readDone := RunControllerLoop(ctx, cfg, wsClient, bipc, dl, writeJSON, uploadSave, downloadSave, &ipcReadyMu, &ipcReady)

	select {
	case <-ctx.Done():
		bhMu.Lock()
		if bhCmd != nil && bhCmd.Process != nil {
			if err := bhCmd.Process.Kill(); err != nil {
				log.Printf("failed to kill BizHawk process: %v", err)
			}
		}
		bhMu.Unlock()
	case <-readDone:
		cancel()
	}
	return nil
}
