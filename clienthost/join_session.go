package clienthost

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/michael4d45/bizshuffle/obslog"
)

const joinConnectTimeout = 30 * time.Second
const joinSettleDelay = 300 * time.Millisecond

// JoinOptions configures a player join session.
type JoinOptions struct {
	ServerURL     string
	PlayerName    string
	OnStatus      func(string)
	OnBizhawkLost func()
}

func joinStatus(opts JoinOptions, msg string) {
	if opts.OnStatus != nil {
		opts.OnStatus(msg)
	}
}

// JoinSession runs BizHawk + WebSocket player client until Stop.
type JoinSession struct {
	dataDir      string
	cancel       context.CancelFunc
	bhController *BizHawkController
	wsClient     *WSClient
	bipc         *BizhawkIPC
}

// StartJoinSession connects as a player after dependencies are satisfied.
func StartJoinSession(parent context.Context, dataDir string, opts JoinOptions) (*JoinSession, error) {
	if err := os.Chdir(dataDir); err != nil {
		return nil, fmt.Errorf("chdir data dir: %w", err)
	}
	if err := EnsureDataDirs(dataDir); err != nil {
		return nil, err
	}
	joinStatus(opts, "Checking dependencies…")
	if err := AssertPlayReady(dataDir); err != nil {
		return nil, err
	}
	if opts.ServerURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if opts.PlayerName == "" {
		return nil, fmt.Errorf("player name is required")
	}

	joinStatus(opts, "Checking BizHawk…")
	exePath, err := ResolveEmuHawkPath(dataDir)
	if err != nil {
		return nil, err
	}
	luaPath, err := EnsureServerLua(dataDir)
	if err != nil {
		return nil, err
	}

	cfg, err := LoadConfig(dataDir)
	if err != nil {
		return nil, err
	}
	if err := cfg.EnsureDefaults(); err != nil {
		return nil, err
	}
	cfg["data_dir"] = dataDir
	cfg["bizhawk_path"] = exePath
	cfg["name"] = opts.PlayerName
	cfg["server"] = opts.ServerURL
	if err := SaveConfig(dataDir, cfg); err != nil {
		return nil, err
	}

	joinStatus(opts, "Reserving Lua IPC port…")
	bipc, err := NewBizhawkIPC(dataDir)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{Timeout: 0}
	wsURL, serverHTTP, err := BuildWSAndHTTP(opts.ServerURL, cfg)
	if err != nil {
		return nil, err
	}
	obslog.Event(obslog.Join, "start", map[string]string{
		"server_url":  opts.ServerURL,
		"http_base":   serverHTTP,
		"ws_url":      wsURL,
		"player":      opts.PlayerName,
		"lua_port":    strconv.Itoa(bipc.Port()),
		"data_dir":    dataDir,
	})
	api := NewAPI(serverHTTP, httpClient, cfg)
	bhController := NewBizHawkController(api, httpClient, cfg, bipc, nil)
	bhController.initialized = true
	bhController.onBizhawkLost = opts.OnBizhawkLost

	wsClient := NewWSClient(wsURL, api, bipc)
	bhController.wsClient = wsClient
	bhController.api = api
	bhController.bipc = bipc

	ctx, cancel := context.WithCancel(parent)
	bhController.SetOnBizhawkReady(func() {
		if ctrl := wsClient.GetController(); ctrl != nil {
			ctrl.OnBizhawkReady(ctx)
		}
	})

	session := &JoinSession{
		dataDir:      dataDir,
		cancel:       cancel,
		bhController: bhController,
		wsClient:     wsClient,
		bipc:         bipc,
	}

	if err := bipc.Start(ctx); err != nil {
		session.Stop()
		return nil, err
	}
	bhController.StartIPCGoroutine(ctx)

	joinStatus(opts, "Launching BizHawk…")
	go func() {
		if err := bhController.LaunchAndManage(ctx, cancel); err != nil {
			fmt.Fprintf(os.Stderr, "LaunchAndManage: %v\n", err)
		}
	}()

	_ = luaPath

	pluginSync := NewPluginSyncManager(api, httpClient, cfg)
	_, _ = pluginSync.SyncPlugins()

	joinStatus(opts, fmt.Sprintf("Joining %s as %s…", opts.ServerURL, opts.PlayerName))
	helloDone := make(chan struct{})
	go func() {
		wsClient.Start(ctx, cfg)
		close(helloDone)
	}()
	select {
	case <-helloDone:
	case <-time.After(joinConnectTimeout):
		session.Stop()
		return nil, fmt.Errorf("timeout waiting for server connection")
	case <-ctx.Done():
		session.Stop()
		return nil, ctx.Err()
	}

	joinStatus(opts, fmt.Sprintf("Connected as %s", opts.PlayerName))
	return session, nil
}

// Stop shuts down the join session.
func (s *JoinSession) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.wsClient != nil {
		s.wsClient.Stop()
	}
	if s.bipc != nil {
		_ = s.bipc.Close()
	}
	if s.bhController != nil {
		s.bhController.Terminate()
	}
}

// StopJoinSession stops a session after a brief settle delay (for re-join).
func StopJoinSession(s *JoinSession) {
	if s == nil {
		return
	}
	s.Stop()
	time.Sleep(joinSettleDelay)
}

// PortFilePath returns the lua port file path under dataDir.
func PortFilePath(dataDir string) string {
	return filepath.Join(dataDir, "lua_server_port.txt")
}
