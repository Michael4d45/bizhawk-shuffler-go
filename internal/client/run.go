package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
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
	// Wrap cancel so we can log a stacktrace when the top-level context is
	// cancelled. This makes it easy to determine which goroutine/code path
	// invoked cancel().
	origCancel := cancel
	// make cancel a guarded logger so accidental calls from internal
	// components won't cancel the whole client. Use origCancel() explicitly
	// in places where we intentionally want to shutdown (signal handler,
	// readDone branch below).
	cancel = func() {
		pcs := make([]uintptr, 8)
		n := runtime.Callers(2, pcs)
		caller := "unknown"
		if n > 0 {
			frames := runtime.CallersFrames(pcs[:n])
			if f, ok := frames.Next(); ok {
				caller = fmt.Sprintf("%s %s:%d", f.Function, f.File, f.Line)
			}
		}
		buf := make([]byte, 1<<12)
		m := runtime.Stack(buf, false)
		log.Printf("top-level cancel() invoked (guarded) by %s; stack snapshot:\n%s", caller, string(buf[:m]))
		// DO NOT call origCancel() here to avoid accidental shutdowns.
	}
	defer cancel()

	// Broad debug: record that Run started with guarded cancel installed.
	log.Printf("run: starting with guarded cancel; wsURL=%s serverHTTP=%s player=%s goroutines=%d", wsURL, serverHTTP, cfg["name"], runtime.NumGoroutine())

	// Defer an exit snapshot so we can see when Run returns and what the
	// goroutine/stack state looked like.
	defer func() {
		buf := make([]byte, 1<<12)
		m := runtime.Stack(buf, true)
		log.Printf("run: exiting; goroutines=%d; stack snapshot:\n%s", runtime.NumGoroutine(), string(buf[:m]))
	}()

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
		// Read the full body and decode the new envelope shape only.
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("failed to read /state.json body: %v", err)
			return running, playerGame
		}

		var env struct {
			State struct {
				Running bool                      `json:"running"`
				Players map[string]map[string]any `json:"players"`
			} `json:"state"`
			Ephemeral map[string]string `json:"ephemeral"`
		}

		if err := json.Unmarshal(data, &env); err != nil {
			log.Printf("failed to decode /state.json envelope: %v", err)
			return running, playerGame
		}

		running = env.State.Running
		if env.State.Players != nil {
			if p, ok := env.State.Players[cfg["name"]]; ok {
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

	StartIPCGoroutine(ctx, bipc, cfg["name"], fetchServerState, &ipcReadyMu, &ipcReady)

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
	// Debug: log configured bizhawk_path before attempting to start
	log.Printf("Debug: configured bizhawk_path=%q", cfg["bizhawk_path"])
	bh, err := StartBizHawk(ctx, cfg, httpClient)
	if err != nil {
		// Treat failure to start BizHawk as fatal for this client.
		// Persist any config changes, call the original cancel to perform
		// a proper shutdown of child components, and return the error.
		if jb, _ := json.MarshalIndent(cfg, "", "  "); jb != nil {
			_ = os.WriteFile(cfgFile, jb, 0644)
			log.Printf("persisted config after launching BizHawk (error path)")
		}
		log.Printf("StartBizHawk failed (fatal): %v", err)
		// call origCancel to actually cancel the top-level context
		origCancel()
		return fmt.Errorf("StartBizHawk failed: %w", err)
	}
	bhCmd = bh
	if jb, _ := json.MarshalIndent(cfg, "", "  "); jb != nil {
		_ = os.WriteFile(cfgFile, jb, 0644)
		log.Printf("persisted config after launching BizHawk")
	}
	if bhCmd != nil {
		log.Printf("monitoring BizHawk pid=%d", bhCmd.Process.Pid)
		MonitorProcess(bhCmd, func(err error) {
			// BizHawk exited. Cancel the top-level context so the client will
			// perform its normal shutdown sequence (terminate remaining
			// components and exit). This makes the client exit when the
			// monitored BizHawk process terminates.
			log.Printf("MonitorProcess: BizHawk pid=%d exited with err=%v; cancelling client", bhCmd.Process.Pid, err)
			origCancel()
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
			log.Printf("terminating BizHawk due to signal: %v", s)
			TerminateProcess(&bhCmd, &bhMu, 3*time.Second)
			log.Printf("signal handler: calling origCancel() after TerminateProcess")
			origCancel()
		}
	}()
	// construct controller and read loop
	_ = RunControllerLoop(ctx, cfg, wsClient, bipc, dl, writeJSON, uploadSave, downloadSave, &ipcReadyMu, &ipcReady)

	// Do not exit simply because the controller read loop ended or an
	// internal component calls the guarded cancel(). Exit when either the
	// top-level context is cancelled (origCancel was invoked by the signal
	// handler or another intended shutdown path) or an OS signal is
	// received. Previously this code blocked on the same `sigs` channel that
	// the signal handler goroutine also read from, which meant the handler
	// consumed the first Ctrl+C and the main goroutine waited for a second
	// one. Use a select so a single Ctrl+C (handled by the goroutine which
	// calls `origCancel()`) will let Run continue.
	select {
	case <-ctx.Done():
		log.Printf("shutdown: context cancelled; terminating BizHawk and exiting")
	case s := <-sigs:
		log.Printf("received shutdown signal: %v; terminating BizHawk and exiting", s)
	}
	bhMu.Lock()
	if bhCmd != nil && bhCmd.Process != nil {
		if err := bhCmd.Process.Kill(); err != nil {
			log.Printf("failed to kill BizHawk process: %v", err)
		}
	}
	bhMu.Unlock()
	return nil
}
