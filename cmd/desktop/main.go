package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/michael4d45/bizshuffle/clienthost"
	"github.com/michael4d45/bizshuffle/cmd/desktop/fyneapp"
	"github.com/michael4d45/bizshuffle/cmd/desktop/hostsession"
	"github.com/michael4d45/bizshuffle/cmd/desktop/updates"
	"github.com/michael4d45/bizshuffle/protocol"
)

func main() {
	dataDir, err := clienthost.DefaultDataDir()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.Chdir(dataDir); err != nil {
		log.Fatal(err)
	}
	logFile := setupFileLogging(dataDir)
	if logFile != nil {
		defer logFile.Close()
	}

	var hostSess hostsession.Session
	var discoveryListener *clienthost.DiscoveryListener
	var discoveryCancel context.CancelFunc
	var joinSession *clienthost.JoinSession
	var joinMu sync.Mutex

	startDiscovery := func() {
		if discoveryListener != nil {
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		discoveryCancel = cancel
		discoveryListener = clienthost.NewDiscoveryListener(protocol.GetDefaultDiscoveryConfig())
		if err := discoveryListener.Start(ctx); err != nil {
			log.Printf("discovery: %v", err)
		}
	}
	startDiscovery()

	fyneapp.Run(fyneapp.Options{
		DataDir:      dataDir,
		LoadSettings: func() clienthost.ShellSettings { return clienthost.LoadShellSettings(dataDir) },
		SaveSettings: func(bindHost, serverURL, playerName string, hostPort int) {
			clienthost.SaveShellSettingsForm(dataDir, bindHost, serverURL, playerName, hostPort)
		},
		VersionLabel: func() string {
			return updates.VersionLabel(updates.State{Version: updates.Version})
		},
		CheckUpdates: func(ctx context.Context) (fyneapp.UpdateInfo, error) {
			st, err := updates.CheckLatest(ctx, updates.DefaultRepo, updates.Version, nil)
			if err != nil {
				return fyneapp.UpdateInfo{}, err
			}
			return fyneapp.UpdateInfo{
				Label:       updates.VersionLabel(st),
				Available:   st.UpdateAvailable,
				DownloadURL: st.DownloadURL,
			}, nil
		},
		OpenDataDir: func() { openPath(dataDir) },
		StartServer: func(host string, port int) (adminURL string, bindHost string, hostPort int, stop func(), err error) {
			res, err := hostSess.Start(context.Background(), host, port)
			if err != nil {
				return "", "", 0, nil, err
			}
			stopFn := func() { _ = hostSess.Stop() }
			return res.AdminURL, res.BindHost, res.HostPort, stopFn, nil
		},
		StopServer:  func() { _ = hostSess.Stop() },
		HostedURL:   func() string { return hostSess.HostedURL() },
		OpenBrowser: openBrowser,
		StartJoin: func(ctx context.Context, serverURL, playerName string, onStatus, onLost func(string)) (*clienthost.JoinSession, error) {
			joinMu.Lock()
			if joinSession != nil {
				clienthost.StopJoinSession(joinSession)
				joinSession = nil
			}
			joinMu.Unlock()
			opts := clienthost.JoinOptions{
				ServerURL:  serverURL,
				PlayerName: playerName,
				OnStatus:   onStatus,
				OnBizhawkLost: func() {
					if onLost != nil {
						onLost("BizHawk closed — disconnected from server")
					}
				},
			}
			sess, err := clienthost.StartJoinSession(ctx, dataDir, opts)
			if err != nil {
				return nil, err
			}
			joinMu.Lock()
			joinSession = sess
			joinMu.Unlock()
			return sess, nil
		},
		StopJoin: func() {
			joinMu.Lock()
			if joinSession != nil {
				clienthost.StopJoinSession(joinSession)
				joinSession = nil
			}
			joinMu.Unlock()
		},
		DepsSnapshot:   clienthost.GetDependenciesSnapshot,
		InstallDep:     clienthost.InstallDependency,
		InstallAllDeps: clienthost.InstallAllDependencies,
		Discover: func() ([]fyneapp.DiscoveredServer, error) {
			if discoveryListener == nil {
				return nil, nil
			}
			raw := discoveryListener.GetDiscoveredServers()
			hosted := hostSess.HostedURL()
			label := "This session"
			if hostSess.IsRunning() {
				label = "Hosted session"
			}
			merged := clienthost.MergeDiscoveredServers(raw, hosted, label)
			out := make([]fyneapp.DiscoveredServer, 0, len(merged))
			for _, e := range merged {
				lbl := e.Label
				if e.IsHosted {
					lbl += " (hosting)"
				}
				out = append(out, fyneapp.DiscoveredServer{Label: lbl, URL: e.URL})
			}
			return out, nil
		},
	})

	joinMu.Lock()
	if joinSession != nil {
		joinSession.Stop()
	}
	joinMu.Unlock()
	_ = hostSess.Stop()
	if discoveryCancel != nil {
		discoveryCancel()
	}
	if discoveryListener != nil {
		_ = discoveryListener.Stop()
	}
}

// setupFileLogging sends standard library log output to dataDir/desktop.log.
// On Windows release builds use -H windowsgui so there is no console for stderr.
func setupFileLogging(dataDir string) *os.File {
	path := filepath.Join(dataDir, "desktop.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("logging: cannot open %s: %v", path, err)
		return nil
	}
	log.SetOutput(f)
	log.Printf("logging to %s", path)
	return f
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("cmd", "/c", "start", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = exec.Command("xdg-open", url).Start()
	}
	if err != nil {
		log.Printf("open browser: %v", err)
	}
}

func openPath(path string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("explorer", path).Start()
	case "darwin":
		err = exec.Command("open", path).Start()
	default:
		err = exec.Command("xdg-open", path).Start()
	}
	if err != nil {
		log.Printf("open folder: %v", err)
	}
}
