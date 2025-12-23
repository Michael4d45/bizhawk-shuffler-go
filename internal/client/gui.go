package client

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// GUI represents the graphical user interface for the client.
type GUI struct {
	app    fyne.App
	window fyne.Window
	client *Client

	// UI Components
	serverAddr      *widget.Entry
	connectBtn      *widget.Button
	statusDot       *canvas.Circle
	statusText      *widget.Label
	autoOpenCheck   *widget.Check
	installVer      *widget.Label
	latestVer       *widget.Label
	updateBtn       *widget.Button
	launchBtn       *widget.Button
	restartBtn      *widget.Button
	playerName      *widget.Label
	connectedStatus *widget.Label
	currentGame     *widget.Label
	instanceID      *widget.Label
	pendingFile     *widget.Label

	ctx    context.Context
	cancel context.CancelFunc
}

// NewGUI creates a new GUI instance.
func NewGUI(c *Client, ctx context.Context, cancel context.CancelFunc) *GUI {
	a := app.NewWithID("com.bizshuffle.client")
	w := a.NewWindow("BizShuffle Client")
	w.Resize(fyne.NewSize(500, 600))

	g := &GUI{
		app:    a,
		window: w,
		client: c,
		ctx:    ctx,
		cancel: cancel,
	}

	g.setupUI()
	return g
}

func (g *GUI) setupUI() {
	// Header: Server Connection
	g.serverAddr = widget.NewEntry()
	g.serverAddr.SetText(g.client.cfg["server"])
	g.serverAddr.PlaceHolder = "http://127.0.0.1:8080"

	g.statusDot = canvas.NewCircle(color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	g.statusDot.Resize(fyne.NewSize(12, 12))
	g.statusText = widget.NewLabel("Disconnected")

	g.connectBtn = widget.NewButton("Connect", func() {
		g.toggleConnection()
	})

	serverSection := container.NewVBox(
		widget.NewLabelWithStyle("Server Connection", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, g.connectBtn, g.serverAddr),
		container.NewHBox(layout.NewSpacer(), g.statusDot, g.statusText),
	)

	// Configuration
	g.autoOpenCheck = widget.NewCheck("Open BizHawk automatically", func(val bool) {
		g.client.cfg.SetBool("auto_open_bizhawk", val)
		_ = g.client.cfg.Save()
	})
	g.autoOpenCheck.Checked = g.client.cfg.GetBool("auto_open_bizhawk")

	configSection := container.NewVBox(
		widget.NewLabelWithStyle("Configuration", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		g.autoOpenCheck,
	)

	// BizHawk Management
	g.installVer = widget.NewLabel("Installed: " + g.client.bhController.GetInstalledVersion())
	g.latestVer = widget.NewLabel("Latest: Checking...")
	g.updateBtn = widget.NewButton("Update BizHawk", func() {
		g.showUpdateWarning()
	})
	g.launchBtn = widget.NewButton("Launch BizHawk", func() {
		if g.client.bipc.IsBizhawkLaunched() {
			g.closeBizHawk()
		} else {
			g.launchBizHawk()
		}
	})
	g.restartBtn = widget.NewButton("Restart BizHawk", func() {
		g.restartBizHawk()
	})

	bizhawkSection := container.NewVBox(
		widget.NewLabelWithStyle("BizHawk Management", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(g.installVer, layout.NewSpacer(), g.latestVer),
		container.NewHBox(g.launchBtn, g.restartBtn, g.updateBtn),
	)

	// Quick Actions
	openRomsBtn := widget.NewButton("Open ROMs", func() {
		g.openFolder("./roms")
	})
	openSavesBtn := widget.NewButton("Open Saves", func() {
		g.openFolder("./saves")
	})

	actionsSection := container.NewVBox(
		widget.NewLabelWithStyle("Quick Actions", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(openRomsBtn, openSavesBtn),
	)

	// Status Panel
	g.playerName = widget.NewLabel("Player: " + g.client.cfg["name"])
	g.connectedStatus = widget.NewLabel("Status: Offline")
	g.currentGame = widget.NewLabel("Current Game: None")
	g.instanceID = widget.NewLabel("Instance ID: None")
	g.pendingFile = widget.NewLabel("Pending File: No")

	statusSection := container.NewVBox(
		widget.NewLabelWithStyle("Server State", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		g.playerName,
		g.connectedStatus,
		g.currentGame,
		g.instanceID,
		g.pendingFile,
	)

	// Main Layout
	content := container.NewVBox(
		serverSection,
		widget.NewSeparator(),
		configSection,
		widget.NewSeparator(),
		bizhawkSection,
		widget.NewSeparator(),
		actionsSection,
		widget.NewSeparator(),
		statusSection,
	)

	g.window.SetContent(container.NewPadded(content))

	// Start background updates
	go g.updateLoop()
}

func (g *GUI) toggleConnection() {
	connected, _ := g.client.wsClient.GetConnectionStatus()

	if connected {
		// Disconnect from server
		g.client.wsClient.Stop()
	} else {
		// Connect to server
		g.connectToServer()
	}
}

func (g *GUI) connectToServer() {
	// Update server in config if it changed
	addr := g.serverAddr.Text
	if addr != g.client.cfg["server"] {
		g.client.cfg["server"] = addr
		_ = g.client.cfg.Save()

		// Rebuild URLs
		wsURL, serverHTTP, err := BuildWSAndHTTP(addr, g.client.cfg)
		if err != nil {
			dialog.ShowError(err, g.window)
			return
		}

		// Update API and WSClient with new URLs
		g.client.api.BaseURL = serverHTTP
		g.client.wsClient.wsURL = wsURL
	}

	// Create new context for connection
	if g.ctx == nil || g.ctx.Err() != nil {
		g.ctx, g.cancel = context.WithCancel(context.Background())
	}

	// Start websocket client
	go g.client.wsClient.Start(g.ctx, g.client.cfg)
}

func (g *GUI) showUpdateWarning() {
	dialog.ShowConfirm("Update BizHawk",
		"This will close BizHawk if it's open and download the latest version.\nYour config.ini and Firmware directory will be preserved.\n\nAre you sure you want to continue?",
		func(ok bool) {
			if ok {
				g.performUpdate()
			}
		}, g.window)
}

func (g *GUI) performUpdate() {
	g.updateBtn.Disable()
	progressBar := widget.NewProgressBar()
	progressBar.SetValue(0)
	progress := dialog.NewCustomWithoutButtons("Updating BizHawk", container.NewVBox(
		widget.NewLabel("Please wait..."),
		progressBar,
	), g.window)
	progress.Show()

	go func() {
		err := g.client.bhController.UpdateBizHawk(func(msg string) {
			log.Printf("BizHawk Update: %s", msg)
		})
		progress.Hide()
		g.updateBtn.Enable()

		if err != nil {
			dialog.ShowError(err, g.window)
		} else {
			fyne.Do(func() {
				g.installVer.SetText("Installed: " + g.client.bhController.GetInstalledVersion())
			})
			dialog.ShowInformation("Success", "BizHawk updated successfully", g.window)
		}
	}()
}

func (g *GUI) launchBizHawk() {
	g.launchBizHawkWithRestartMode(false)
}

func (g *GUI) launchBizHawkWithRestartMode(keepRestartMode bool) {
	fyne.Do(func() {
		g.launchBtn.Disable()
		defer g.launchBtn.Enable()
	})

	if g.ctx == nil || g.ctx.Err() != nil {
		log.Printf("GUI: Cannot launch BizHawk - no valid context")
		return
	}

	// Only disable restart mode for normal launches (not config updates)
	if !keepRestartMode {
		g.client.bhController.SetRestartMode(false)
	}

	go func() {
		log.Printf("GUI: Launching BizHawk manually")
		if err := g.client.bhController.LaunchAndManage(g.ctx, g.cancel); err != nil {
			log.Printf("GUI: LaunchAndManage error: %v", err)
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("failed to launch BizHawk: %v", err), g.window)
			})
		}
	}()
}

func (g *GUI) closeBizHawk() {
	log.Printf("GUI: Closing BizHawk manually")

	// Enable restart mode so BizHawk exit doesn't shut down the client
	g.client.bhController.SetRestartMode(true)

	// Cancel the context to terminate BizHawk
	if g.ctx != nil && g.ctx.Err() == nil {
		g.cancel()
		// Create new context for potential future launches
		g.ctx, g.cancel = context.WithCancel(context.Background())
	}
}

func (g *GUI) terminateBizHawkForConfig() {
	log.Printf("GUI: Terminating BizHawk for config update")

	// Enable restart mode so BizHawk exit doesn't shut down the client
	g.client.bhController.SetRestartMode(true)

	// Reset IPC state to ensure clean reconnection for new BizHawk
	g.client.bipc.Reset()

	// Terminate BizHawk process directly without cancelling client context
	g.client.bhController.Terminate()

	// Wait a bit for the old process's MonitorProcess callback to run
	// This ensures restartMode stays true when the callback checks it
	time.Sleep(200 * time.Millisecond)
}

func (g *GUI) setBizHawkRestartMode(mode bool) {
	log.Printf("GUI: Setting BizHawk restart mode to %v", mode)
	g.client.bhController.SetRestartMode(mode)
}

func (g *GUI) restartBizHawk() {
	log.Printf("GUI: Restarting BizHawk")

	// Check if BizHawk is running
	wasRunning := g.client.bipc.IsBizhawkLaunched()

	// Close BizHawk if it's running before restart
	if wasRunning {
		log.Printf("GUI: Terminating BizHawk before restart")
		g.terminateBizHawkForConfig()
	}

	// Small delay to allow BizHawk process cleanup before relaunch
	time.Sleep(500 * time.Millisecond)

	// Launch BizHawk again (preserving restart mode)
	if wasRunning {
		log.Printf("GUI: Launching BizHawk after restart")
		fyne.Do(func() {
			g.launchBizHawkWithRestartMode(true)
		})
	} else {
		log.Printf("GUI: BizHawk was not running - start BizHawk to apply restart")
	}
}

func (g *GUI) openFolder(path string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		log.Printf("Failed to get absolute path: %v", err)
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", absPath)
	case "darwin":
		cmd = exec.Command("open", absPath)
	default: // Linux and others
		cmd = exec.Command("xdg-open", absPath)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open folder: %v", err)
	}
}

func (g *GUI) updateLoop() {
	// Check for latest version once at startup
	go func() {
		ver, err := g.client.bhController.GetLatestVersion()
		fyne.Do(func() {
			if err == nil {
				g.latestVer.SetText("Latest: " + ver)
			} else {
				g.latestVer.SetText("Latest: Error")
			}
		})
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		g.refreshStatus()
	}
}

func (g *GUI) refreshStatus() {
	connected, bizhawkReady := g.client.wsClient.GetConnectionStatus()

	// Update server state from controller
	ctrl := g.client.wsClient.GetController()
	var game, iid, pending string
	if ctrl != nil {
		game, iid, pending = ctrl.GetState()
		if game == "" {
			game = "None"
		}
		if iid == "" {
			iid = "None"
		}
		// Set BizHawk control callbacks on controller for config updates (only once)
		ctrl.SetRestartBizhawkCallback(g.restartBizHawk)
		ctrl.SetBizhawkCallbacks(g.closeBizHawk, g.terminateBizHawkForConfig, g.launchBizHawk, func() { g.launchBizHawkWithRestartMode(true) }, g.setBizHawkRestartMode)
	} else {
		game = "None"
		iid = "None"
		pending = ""
	}

	// Check if BizHawk is launched
	bizhawkLaunched := g.client.bipc.IsBizhawkLaunched()

	// Marshal all UI updates to the main UI thread
	fyne.Do(func() {
		if connected {
			if bizhawkReady {
				g.statusDot.FillColor = color.NRGBA{R: 0, G: 255, B: 0, A: 255}
				g.statusText.SetText("Connected (BizHawk Ready)")
				g.connectBtn.SetText("Disconnect")
				g.connectedStatus.SetText("Status: Online (BizHawk Ready)")
			} else {
				g.statusDot.FillColor = color.NRGBA{R: 255, G: 165, B: 0, A: 255} // Orange
				g.statusText.SetText("Connected (BizHawk Not Ready)")
				g.connectBtn.SetText("Disconnect")
				g.connectedStatus.SetText("Status: Online (BizHawk Not Ready)")
			}
		} else {
			g.statusDot.FillColor = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
			g.statusText.SetText("Disconnected")
			g.connectBtn.SetText("Connect")
			g.connectedStatus.SetText("Status: Offline")
		}
		g.statusDot.Refresh()

		// Update launch button text and restart button state based on BizHawk status
		if bizhawkLaunched {
			g.launchBtn.SetText("Close BizHawk")
			g.restartBtn.Enable()
		} else {
			g.launchBtn.SetText("Launch BizHawk")
			g.restartBtn.Disable()
		}

		g.currentGame.SetText("Current Game: " + game)
		g.instanceID.SetText("Instance ID: " + iid)
		if pending != "" {
			g.pendingFile.SetText("Pending File: Yes (" + pending + ")")
		} else {
			g.pendingFile.SetText("Pending File: No")
		}
	})
}

// Show starts the GUI and blocks until the window is closed.
func (g *GUI) Show() {
	g.window.ShowAndRun()
}
