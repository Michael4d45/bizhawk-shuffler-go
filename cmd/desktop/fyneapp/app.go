package fyneapp

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/michael4d45/bizshuffle/clienthost"
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
	w := a.NewWindow("BizShuffle")
	w.Resize(fyne.NewSize(540, 580))

	var serverStop func()
	var installing bool
	var busy bool
	var depsChecking = true
	var saveTimer *time.Timer
	var saveMu sync.Mutex

	status := widget.NewLabel("Host a session or join one as a player.")
	hostEntry := widget.NewEntry()
	portEntry := widget.NewEntry()
	serverURLEntry := widget.NewEntry()
	serverURLEntry.SetPlaceHolder("http://127.0.0.1:8080")
	playerNameEntry := widget.NewEntry()
	playerNameEntry.SetPlaceHolder("Player name")
	depsBox := container.NewVBox()
	versionLabel := widget.NewLabel("")
	updateBtn := widget.NewButton("Update", nil)
	updateBtn.Hide()

	var discovered []DiscoveredServer
	discoveredList := widget.NewList(
		func() int { return len(discovered) },
		func() fyne.CanvasObject { return widget.NewButton("", nil) },
		func(i int, o fyne.CanvasObject) {
			if i >= len(discovered) {
				return
			}
			o.(*widget.Button).SetText(discovered[i].Label + " — " + discovered[i].URL)
		},
	)

	joinBtn := widget.NewButton("Join", nil)
	hostBtn := widget.NewButton("Host (server + admin)", nil)
	stopHostBtn := widget.NewButton("Stop host", nil)
	stopHostBtn.Hide()

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
			port, _ := strconv.Atoi(portEntry.Text)
			fyne.Do(func() {
				opts.SaveSettings(hostEntry.Text, serverURLEntry.Text, playerNameEntry.Text, port)
			})
		})
	}

	applySettings := func() {
		if opts.LoadSettings == nil {
			return
		}
		s := opts.LoadSettings()
		hostEntry.SetText(s.BindHost)
		portEntry.SetText(strconv.Itoa(s.HostPort))
		serverURLEntry.SetText(s.ServerURL)
		playerNameEntry.SetText(s.PlayerName)
	}

	var refreshDeps func()
	var refreshDiscovery func()

	updateJoinEnabled := func() {
		blocked := depsChecking
		if !blocked && opts.DepsSnapshot != nil {
			blocked = opts.DepsSnapshot(opts.DataDir).PlayBlocked
		}
		if installing || blocked || busy {
			joinBtn.Disable()
		} else {
			joinBtn.Enable()
		}
		if busy {
			hostBtn.Disable()
		} else {
			hostBtn.Enable()
		}
	}

	refreshDeps = func() {
		if opts.DepsSnapshot == nil {
			depsChecking = false
			depsBox.Objects = nil
			updateJoinEnabled()
			return
		}
		if depsChecking {
			depsBox.Objects = []fyne.CanvasObject{widget.NewLabel("Checking dependencies…")}
			depsBox.Refresh()
			updateJoinEnabled()
		}
		snap := opts.DepsSnapshot(opts.DataDir)
		depsChecking = false
		var rows []fyne.CanvasObject
		if len(snap.Items) >= 2 && opts.InstallAllDeps != nil {
			rows = append(rows, widget.NewButton("Install all", func() {
				if installing || busy {
					return
				}
				installing = true
				updateJoinEnabled()
				go func() {
					err := opts.InstallAllDeps(opts.DataDir, func(msg string) {
						fyne.Do(func() { status.SetText(msg) })
					})
					installing = false
					fyne.Do(func() {
						if err != nil {
							status.SetText("Install failed: " + err.Error())
						} else {
							status.SetText("Install complete")
						}
						refreshDeps()
					})
				}()
			}))
		}
		for _, item := range snap.Items {
			it := item
			btn := widget.NewButton(it.ActionLabel, func() {
				if installing || busy || opts.InstallDep == nil {
					return
				}
				installing = true
				updateJoinEnabled()
				status.SetText("Installing " + string(it.ID) + "…")
				go func() {
					err := opts.InstallDep(opts.DataDir, it.ID, func(msg string) {
						fyne.Do(func() { status.SetText(msg) })
					})
					installing = false
					fyne.Do(func() {
						if err != nil {
							status.SetText("Install failed: " + err.Error())
						} else {
							status.SetText("Install complete")
						}
						refreshDeps()
					})
				}()
			})
			rows = append(rows, widget.NewLabel(it.Label+": "+it.Detail), btn)
		}
		if snap.PlayBlocked && len(snap.Items) > 0 {
			rows = append(rows, widget.NewLabel(clienthost.PlayBlockedMessage(snap)))
		}
		depsBox.Objects = rows
		depsBox.Refresh()
		updateJoinEnabled()
	}

	onFieldChange := func() {
		scheduleSave()
	}
	hostEntry.OnChanged = func(string) { onFieldChange() }
	portEntry.OnChanged = func(string) { onFieldChange() }
	serverURLEntry.OnChanged = func(string) { onFieldChange() }
	playerNameEntry.OnChanged = func(string) { onFieldChange() }

	hostBtn.OnTapped = func() {
		if busy {
			return
		}
		opts.StopJoin()
		if serverStop != nil {
			serverStop()
			serverStop = nil
		}
		port, _ := strconv.Atoi(portEntry.Text)
		busy = true
		updateJoinEnabled()
		status.SetText("Starting host…")
		go func() {
			adminURL, bindHost, hostPort, stop, err := opts.StartServer(hostEntry.Text, port)
			fyne.Do(func() {
				busy = false
				if err != nil {
					status.SetText("Host failed: " + err.Error())
					updateJoinEnabled()
					return
				}
				serverStop = stop
				hostEntry.SetText(bindHost)
				portEntry.SetText(strconv.Itoa(hostPort))
				scheduleSave()
				joinURL := fmt.Sprintf("http://127.0.0.1:%d", hostPort)
				if serverURLEntry.Text == "" {
					serverURLEntry.SetText(joinURL)
					scheduleSave()
				}
				stopHostBtn.Show()
				status.SetText(fmt.Sprintf("Hosting at %s (listening on %s:%d)", adminURL, bindHost, hostPort))
				if opts.OpenBrowser != nil {
					opts.OpenBrowser(adminURL)
				}
				updateJoinEnabled()
				refreshDiscovery()
			})
		}()
	}

	stopHostBtn.OnTapped = func() {
		if serverStop != nil {
			serverStop()
			serverStop = nil
		}
		if opts.StopServer != nil {
			opts.StopServer()
		}
		stopHostBtn.Hide()
		status.SetText("Host stopped")
		refreshDiscovery()
	}

	joinBtn.OnTapped = func() {
		if opts.StartJoin == nil {
			status.SetText("Join not configured")
			return
		}
		serverURL := serverURLEntry.Text
		playerName := playerNameEntry.Text
		if serverURL == "" || playerName == "" {
			status.SetText("Server URL and player name are required")
			return
		}
		if snap := opts.DepsSnapshot(opts.DataDir); snap.PlayBlocked {
			status.SetText(clienthost.PlayBlockedMessage(snap))
			return
		}
		scheduleSave()
		busy = true
		updateJoinEnabled()
		go func() {
			onStatus := func(msg string) { fyne.Do(func() { status.SetText(msg) }) }
			onLost := func(msg string) { fyne.Do(func() { status.SetText(msg) }) }
			_, err := opts.StartJoin(context.Background(), serverURL, playerName, onStatus, onLost)
			fyne.Do(func() {
				busy = false
				if err != nil {
					status.SetText("Join failed: " + err.Error())
				} else {
					status.SetText("Joined " + serverURL + " as " + playerName)
				}
				updateJoinEnabled()
			})
		}()
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
		discoveredList.Refresh()
		if serverURLEntry.Text == "" && opts.HostedURL != nil {
			if u := opts.HostedURL(); u != "" {
				serverURLEntry.SetText(u)
				scheduleSave()
			}
		}
	}
	discoveredList.OnSelected = func(id widget.ListItemID) {
		if int(id) < len(discovered) {
			serverURLEntry.SetText(discovered[id].URL)
			scheduleSave()
		}
	}

	updateBtn.OnTapped = func() {
		if opts.CheckUpdates == nil {
			return
		}
		go func() {
			info, err := opts.CheckUpdates(context.Background())
			fyne.Do(func() {
				if err != nil {
					status.SetText("Update check failed: " + err.Error())
					return
				}
				versionLabel.SetText(info.Label)
				if info.Available && info.DownloadURL != "" {
					updateBtn.Show()
					if opts.OpenBrowser != nil {
						opts.OpenBrowser(info.DownloadURL)
					}
				}
			})
		}()
	}

	w.SetContent(container.NewVBox(
		widget.NewLabel("BizShuffle"),
		widget.NewForm(
			widget.NewFormItem("Bind host", hostEntry),
			widget.NewFormItem("Port (0 = free)", portEntry),
		),
		container.NewHBox(hostBtn, stopHostBtn),
		widget.NewSeparator(),
		widget.NewLabel("Join session"),
		widget.NewForm(
			widget.NewFormItem("Server URL", serverURLEntry),
			widget.NewFormItem("Player name", playerNameEntry),
		),
		depsBox,
		joinBtn,
		widget.NewButton("Refresh servers", func() { refreshDiscovery() }),
		discoveredList,
		status,
		container.NewHBox(versionLabel, widget.NewButton("Check updates", func() { updateBtn.OnTapped() })),
		widget.NewButton("Open data folder", func() {
			if opts.OpenDataDir != nil {
				opts.OpenDataDir()
			}
		}),
	))

	applySettings()
	if opts.VersionLabel != nil {
		versionLabel.SetText(opts.VersionLabel())
	}
	refreshDeps()
	refreshDiscovery()

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
