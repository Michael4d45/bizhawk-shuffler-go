package fyneapp

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/widget"

	"github.com/michael4d45/bizshuffle/clienthost"
	"github.com/michael4d45/bizshuffle/cmd/desktop/fyneapp/ui"
)

// UpdateInfo is the app update footer state.
type UpdateInfo struct {
	Label       string
	Available   bool
	DownloadURL string
}

// Options configures the desktop shell.
type Options struct {
	DataDir        string
	LoadSettings   func() clienthost.ShellSettings
	SaveSettings   func(bindHost, serverURL, playerName string, hostPort int)
	VersionLabel   func() string
	CheckUpdates   func(ctx context.Context) (UpdateInfo, error)
	OpenDataDir    func()
	StartServer    func(host string, port int) (adminURL, bindHost string, hostPort int, stop func(), err error)
	StopServer     func()
	HostedURL      func() string
	OpenBrowser    func(url string)
	StartJoin      func(ctx context.Context, serverURL, playerName string, onStatus, onLost func(string)) (*clienthost.JoinSession, error)
	StopJoin       func()
	DepsSnapshot   func(dataDir string) clienthost.DependenciesSnapshot
	InstallDep     func(dataDir string, id clienthost.DependencyID, progress func(string)) error
	InstallAllDeps func(dataDir string, progress func(string)) error
	Discover       func() ([]DiscoveredServer, error)
}

// DiscoveredServer is one LAN discovery entry.
type DiscoveredServer struct {
	Label string
	URL   string
}

// Run starts the BizShuffle desktop shell (Host / Join).
func Run(opts Options) {
	a := app.NewWithID("com.bizshuffle.desktop")
	a.Settings().SetTheme(ui.NewTheme())

	w := a.NewWindow("BizShuffle")
	w.Resize(ui.WindowDefaultSize())

	var discovered []DiscoveredServer
	sh := buildShell(&discovered)
	st := &shellState{
		statusText: "Host a session or join one as a player.",
		statusSev:  ui.StatusSeverityInfo,
	}
	w.SetContent(sh.root)

	var serverStop func()
	var saveTimer *time.Timer
	var saveMu sync.Mutex
	depsBlocked := func() bool {
		if opts.DepsSnapshot == nil {
			return false
		}
		return opts.DepsSnapshot(opts.DataDir).PlayBlocked
	}

	applyUI := func() {
		st.apply(sh, depsBlocked)
	}

	scheduleSave := func() {
		if opts.SaveSettings == nil {
			return
		}
		saveMu.Lock()
		defer saveMu.Unlock()
		if saveTimer != nil {
			saveTimer.Stop()
		}
		saveTimer = time.AfterFunc(400*time.Millisecond, func() {
			port, _ := strconv.Atoi(sh.portEntry.Text)
			fyne.Do(func() {
				opts.SaveSettings(sh.hostEntry.Text, sh.serverURLEntry.Text, sh.playerNameEntry.Text, port)
			})
		})
	}

	applySettings := func() {
		if opts.LoadSettings == nil {
			return
		}
		s := opts.LoadSettings()
		sh.hostEntry.SetText(s.BindHost)
		sh.portEntry.SetText(strconv.Itoa(s.HostPort))
		sh.serverURLEntry.SetText(s.ServerURL)
		sh.playerNameEntry.SetText(s.PlayerName)
	}

	var refreshDeps func()
	var refreshDiscovery func()

	installOne := func(it clienthost.DependencyItem) {
		if st.installing || st.busy || opts.InstallDep == nil {
			return
		}
		st.installing = true
		st.setStatus("Installing "+string(it.ID)+"…", ui.StatusSeverityInfo)
		applyUI()
		go func() {
			err := opts.InstallDep(opts.DataDir, it.ID, func(msg string) {
				fyne.Do(func() {
					st.setStatus(msg, ui.StatusSeverityInfo)
					applyUI()
				})
			})
			st.installing = false
			fyne.Do(func() {
				if err != nil {
					st.setStatus("Install failed: "+err.Error(), ui.StatusSeverityError)
				} else {
					st.setStatus("Install complete", ui.StatusSeveritySuccess)
				}
				refreshDeps()
				applyUI()
			})
		}()
	}

	installAll := func() {
		if st.installing || st.busy || opts.InstallAllDeps == nil {
			return
		}
		st.installing = true
		st.setStatus("Installing dependencies…", ui.StatusSeverityInfo)
		applyUI()
		go func() {
			err := opts.InstallAllDeps(opts.DataDir, func(msg string) {
				fyne.Do(func() {
					st.setStatus(msg, ui.StatusSeverityInfo)
					applyUI()
				})
			})
			st.installing = false
			fyne.Do(func() {
				if err != nil {
					st.setStatus("Install failed: "+err.Error(), ui.StatusSeverityError)
				} else {
					st.setStatus("Install complete", ui.StatusSeveritySuccess)
				}
				refreshDeps()
				applyUI()
			})
		}()
	}

	refreshDeps = func() {
		if opts.DepsSnapshot == nil {
			st.depsChecking = false
			updateDepsPanelVisibility(sh, clienthost.DependenciesSnapshot{}, false)
			applyUI()
			return
		}
		if st.depsChecking {
			renderDepsPanel(sh, clienthost.DependenciesSnapshot{}, true, st.installing, nil, nil)
			updateDepsPanelVisibility(sh, clienthost.DependenciesSnapshot{}, true)
			applyUI()
		}
		snap := opts.DepsSnapshot(opts.DataDir)
		st.depsChecking = false
		var onAll func()
		if len(snap.Items) >= 2 && opts.InstallAllDeps != nil {
			onAll = installAll
		}
		if depsPanelNeeded(snap, false) {
			renderDepsPanel(sh, snap, false, st.installing, onAll, installOne)
		}
		updateDepsPanelVisibility(sh, snap, false)
		applyUI()
	}

	refreshDiscovery = func() {
		if opts.Discover == nil {
			return
		}
		servers, err := opts.Discover()
		if err != nil {
			return
		}
		discovered = servers
		sh.discoveryList.Refresh()
		setDiscoveryEmptyVisible(sh, len(discovered) == 0)
		if sh.serverURLEntry.Text == "" && opts.HostedURL != nil {
			if u := opts.HostedURL(); u != "" {
				sh.serverURLEntry.SetText(u)
				scheduleSave()
			}
		}
	}

	sh.discoveryList.OnSelected = func(id widget.ListItemID) {
		if int(id) < len(discovered) {
			sh.serverURLEntry.SetText(discovered[id].URL)
			scheduleSave()
		}
	}

	onFieldChange := func() { scheduleSave() }
	sh.hostEntry.OnChanged = func(string) { onFieldChange() }
	sh.portEntry.OnChanged = func(string) { onFieldChange() }
	sh.serverURLEntry.OnChanged = func(string) { onFieldChange() }
	sh.playerNameEntry.OnChanged = func(string) { onFieldChange() }

	sh.hostBtn.OnTapped = func() {
		if st.busy {
			return
		}
		port, _ := strconv.Atoi(sh.portEntry.Text)
		st.busy = true
		st.setStatus("Starting host…", ui.StatusSeverityInfo)
		applyUI()
		prevStop := serverStop
		serverStop = nil
		go func() {
			opts.StopJoin()
			if prevStop != nil {
				prevStop()
			}
			adminURL, bindHost, hostPort, stop, err := opts.StartServer(sh.hostEntry.Text, port)
			fyne.Do(func() {
				st.busy = false
				if err != nil {
					st.setStatus("Host failed: "+err.Error(), ui.StatusSeverityError)
					applyUI()
					return
				}
				serverStop = stop
				st.hosting = true
				sh.hostEntry.SetText(bindHost)
				sh.portEntry.SetText(strconv.Itoa(hostPort))
				scheduleSave()
				joinURL := fmt.Sprintf("http://127.0.0.1:%d", hostPort)
				if sh.serverURLEntry.Text == "" {
					sh.serverURLEntry.SetText(joinURL)
					scheduleSave()
				}
				st.setStatus(fmt.Sprintf("Hosting at %s (listening on %s:%d)", adminURL, bindHost, hostPort), ui.StatusSeveritySuccess)
				if opts.OpenBrowser != nil {
					opts.OpenBrowser(adminURL)
				}
				applyUI()
				refreshDiscovery()
			})
		}()
	}

	sh.stopHostBtn.OnTapped = func() {
		if st.busy {
			return
		}
		st.busy = true
		sh.stopHostBtn.Disable()
		sh.hostBtn.Disable()
		st.setStatus("Stopping host…", ui.StatusSeverityInfo)
		applyUI()
		stopFn := serverStop
		serverStop = nil
		go func() {
			defer func() {
				fyne.Do(func() {
					st.busy = false
					st.hosting = false
					sh.stopHostBtn.Enable()
					sh.hostBtn.Enable()
					st.setStatus("Host stopped", ui.StatusSeverityInfo)
					applyUI()
					refreshDiscovery()
				})
			}()
			if stopFn != nil {
				stopFn()
			}
			opts.StopJoin()
		}()
	}

	sh.joinBtn.OnTapped = func() {
		if opts.StartJoin == nil {
			st.setStatus("Join not configured", ui.StatusSeverityError)
			applyUI()
			return
		}
		serverURL := sh.serverURLEntry.Text
		playerName := sh.playerNameEntry.Text
		if serverURL == "" || playerName == "" {
			st.setStatus("Server URL and player name are required", ui.StatusSeverityWarning)
			applyUI()
			return
		}
		if snap := opts.DepsSnapshot(opts.DataDir); snap.PlayBlocked {
			st.setStatus(clienthost.PlayBlockedMessage(snap), ui.StatusSeverityWarning)
			applyUI()
			return
		}
		scheduleSave()
		st.busy = true
		st.setStatus("Joining…", ui.StatusSeverityInfo)
		applyUI()
		go func() {
			onStatus := func(msg string) {
				fyne.Do(func() {
					st.setStatus(msg, ui.StatusSeverityInfo)
					applyUI()
				})
			}
			onLost := func(msg string) {
				fyne.Do(func() {
					st.setStatus(msg, ui.StatusSeverityWarning)
					applyUI()
				})
			}
			_, err := opts.StartJoin(context.Background(), serverURL, playerName, onStatus, onLost)
			fyne.Do(func() {
				st.busy = false
				if err != nil {
					st.setStatus("Join failed: "+err.Error(), ui.StatusSeverityError)
				} else {
					st.setStatus("Joined "+serverURL+" as "+playerName, ui.StatusSeveritySuccess)
				}
				applyUI()
			})
		}()
	}

	sh.refreshDiscoveryBtn.OnTapped = func() { refreshDiscovery() }

	runUpdateCheck := func() {
		if opts.CheckUpdates == nil {
			return
		}
		go func() {
			info, err := opts.CheckUpdates(context.Background())
			fyne.Do(func() {
				if err != nil {
					st.setStatus("Update check failed: "+err.Error(), ui.StatusSeverityError)
					applyUI()
					return
				}
				sh.versionLabel.SetText(info.Label)
				if info.Available && info.DownloadURL != "" {
					sh.updateBtn.Show()
					sh.updateBtn.OnTapped = func() {
						if opts.OpenBrowser != nil {
							opts.OpenBrowser(info.DownloadURL)
						}
					}
				} else {
					sh.updateBtn.Hide()
				}
				applyUI()
			})
		}()
	}

	sh.checkUpdatesBtn.OnTapped = runUpdateCheck
	sh.updateBtn.OnTapped = nil

	sh.openDataBtn.OnTapped = func() {
		if opts.OpenDataDir != nil {
			opts.OpenDataDir()
		}
	}
	sh.openDataBtn.Importance = widget.LowImportance

	applySettings()
	if opts.VersionLabel != nil {
		sh.versionLabel.SetText(opts.VersionLabel())
	}
	st.depsChecking = true
	refreshDeps()
	refreshDiscovery()
	applyUI()

	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for range t.C {
			fyne.Do(refreshDiscovery)
		}
	}()

	w.SetOnClosed(func() {
		opts.StopJoin()
		if serverStop != nil {
			serverStop()
		}
		if opts.StopServer != nil {
			opts.StopServer()
		}
	})

	w.ShowAndRun()
}
