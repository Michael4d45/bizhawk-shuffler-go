package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/michael4d45/bizshuffle/internal/deps"
	"github.com/michael4d45/bizshuffle/internal/installer"
)

func main() {
	myApp := app.NewWithID("com.bizshuffle.installer")
	myWindow := myApp.NewWindow("BizShuffle Installer")
	myWindow.Resize(fyne.NewSize(600, 500))

	var installServer, installClient bool
	var serverDir, clientDir string

	// Get appropriate default directory (avoid system folders when running from zip)
	currentDir := getDefaultInstallDir()

	serverDir = filepath.Join(currentDir, "BizShuffle Server")
	clientDir = filepath.Join(currentDir, "BizShuffle Client")

	// UI Components
	title := widget.NewLabel("BizShuffle Installer")
	title.TextStyle = fyne.TextStyle{Bold: true}

	progressLabel := widget.NewLabel("Select components to install")

	// Get local network IP address for default server host
	defaultHost := getLocalIP()
	if defaultHost == "" {
		defaultHost = "127.0.0.1"
	}

	// Server configuration fields
	serverHostLabel := widget.NewLabel("Server Host:")
	serverHostEntry := widget.NewEntry()
	serverHostEntry.SetText(defaultHost)
	serverHostEntry.SetPlaceHolder(defaultHost)

	serverPortLabel := widget.NewLabel("Server Port:")
	serverPortEntry := widget.NewEntry()
	serverPortEntry.SetText("8080")
	serverPortEntry.SetPlaceHolder("8080")

	serverConfigContainer := container.NewVBox(
		container.NewBorder(nil, nil, serverHostLabel, nil, serverHostEntry),
		container.NewBorder(nil, nil, serverPortLabel, nil, serverPortEntry),
	)
	serverConfigContainer.Hide()

	serverCheck := widget.NewCheck("Install Server", func(checked bool) {
		installServer = checked
		if checked {
			serverConfigContainer.Show()
		} else {
			serverConfigContainer.Hide()
		}
		updateReadyStatus(progressLabel, installServer, installClient)
	})

	// Client configuration fields
	clientServerLabel := widget.NewLabel("Server URL:")
	clientServerEntry := widget.NewEntry()
	clientServerEntry.SetPlaceHolder("http://127.0.0.1:8080")

	clientPlayerLabel := widget.NewLabel("Player Name:")
	clientPlayerEntry := widget.NewEntry()
	clientPlayerEntry.SetPlaceHolder("Player1")

	clientConfigContainer := container.NewVBox(
		container.NewBorder(nil, nil, clientServerLabel, nil, clientServerEntry),
		container.NewBorder(nil, nil, clientPlayerLabel, nil, clientPlayerEntry),
	)
	clientConfigContainer.Hide()

	clientCheck := widget.NewCheck("Install Client", func(checked bool) {
		installClient = checked
		if checked {
			clientConfigContainer.Show()
		} else {
			clientConfigContainer.Hide()
		}
		updateReadyStatus(progressLabel, installServer, installClient)
	})

	serverDirLabel := widget.NewLabel("Server Directory:")
	serverDirEntry := widget.NewEntry()
	serverDirEntry.SetText(serverDir)
	serverDirButton := widget.NewButton("Browse...", func() {
		dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
			if err == nil && dir != nil {
				serverDir = dir.Path()
				serverDirEntry.SetText(serverDir)
			}
		}, myWindow)
	})
	serverDirContainer := container.NewBorder(nil, nil, serverDirLabel, serverDirButton, serverDirEntry)

	serverSection := container.NewVBox(
		serverCheck,
		serverDirContainer,
		serverConfigContainer,
	)

	clientDirLabel := widget.NewLabel("Client Directory:")
	clientDirEntry := widget.NewEntry()
	clientDirEntry.SetText(clientDir)
	clientDirButton := widget.NewButton("Browse...", func() {
		dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
			if err == nil && dir != nil {
				clientDir = dir.Path()
				clientDirEntry.SetText(clientDir)
			}
		}, myWindow)
	})
	clientDirContainer := container.NewBorder(nil, nil, clientDirLabel, clientDirButton, clientDirEntry)

	clientSection := container.NewVBox(
		clientCheck,
		clientDirContainer,
		clientConfigContainer,
	)

	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	logText := widget.NewRichText()
	logText.Wrapping = fyne.TextWrapWord
	logScroll := container.NewScroll(logText)
	logScroll.SetMinSize(fyne.NewSize(0, 200))

	var installButton, closeAndOpenButton *widget.Button
	installButton = widget.NewButton("Install", func() {
		if !installServer && !installClient {
			dialog.ShowError(fmt.Errorf("please select at least one component to install"), myWindow)
			return
		}

		serverDir = serverDirEntry.Text
		clientDir = clientDirEntry.Text

		if (installServer && serverDir == "") || (installClient && clientDir == "") {
			dialog.ShowError(fmt.Errorf("please specify installation directories"), myWindow)
			return
		}

		// Collect configuration values
		var serverHost, serverPort string
		if installServer {
			serverHost = serverHostEntry.Text
			if serverHost == "" {
				serverHost = "127.0.0.1"
			}
			serverPort = serverPortEntry.Text
			if serverPort == "" {
				serverPort = "8080"
			}
		}

		var clientServer, clientPlayerName string
		if installClient {
			clientServer = clientServerEntry.Text
			clientPlayerName = clientPlayerEntry.Text
		}

		// Disable UI during installation
		installButton.Disable()
		serverCheck.Disable()
		clientCheck.Disable()
		serverDirEntry.Disable()
		clientDirEntry.Disable()
		serverDirButton.Disable()
		clientDirButton.Disable()
		serverHostEntry.Disable()
		serverPortEntry.Disable()
		clientServerEntry.Disable()
		clientPlayerEntry.Disable()
		progressBar.Show()
		progressBar.SetValue(0)

		// Run installation in goroutine
		go func() {
			if err := runInstallation(installServer, installClient, serverDir, clientDir,
				serverHost, serverPort, clientServer, clientPlayerName,
				func(msg string) {
					fyne.Do(func() {
						progressLabel.SetText(msg)
						logText.Segments = append(logText.Segments, &widget.TextSegment{Text: msg + "\n"})
						logText.Refresh()
						logScroll.ScrollToBottom()
					})
				}, func(val float64) {
					fyne.Do(func() {
						progressBar.SetValue(val)
					})
				}); err != nil {
				fyne.Do(func() {
					dialog.ShowError(err, myWindow)
				})
			} else {
				fyne.Do(func() {
					progressLabel.SetText("Installation complete!")
					progressBar.SetValue(1.0)
					dialog.ShowInformation("Success", "BizShuffle has been installed successfully!", myWindow)
					closeAndOpenButton.Show()
				})
			}

			// Re-enable UI
			fyne.Do(func() {
				installButton.Enable()
				serverCheck.Enable()
				clientCheck.Enable()
				serverDirEntry.Enable()
				clientDirEntry.Enable()
				serverDirButton.Enable()
				clientDirButton.Enable()
				serverHostEntry.Enable()
				serverPortEntry.Enable()
				clientServerEntry.Enable()
				clientPlayerEntry.Enable()
			})
		}()
	})

	// Close and Open Folder button - initially hidden
	closeAndOpenButton = widget.NewButton("Close and Open Folder", func() {
		// Determine which folder to open (prefer server if both installed)
		folderToOpen := ""
		if installServer {
			folderToOpen = serverDir
		} else if installClient {
			folderToOpen = clientDir
		}

		if folderToOpen != "" {
			openFolder(folderToOpen)
		}

		// Close the installer window
		myWindow.Close()
	})
	closeAndOpenButton.Hide()

	content := container.NewVBox(
		title,
		widget.NewSeparator(),
		serverSection,
		widget.NewSeparator(),
		clientSection,
		widget.NewSeparator(),
		progressLabel,
		progressBar,
		widget.NewLabel("Log:"),
		logScroll,
		widget.NewSeparator(),
		installButton,
		closeAndOpenButton,
	)

	myWindow.SetContent(container.NewPadded(content))
	myWindow.ShowAndRun()
}

func updateReadyStatus(label *widget.Label, installServer, installClient bool) {
	if installServer || installClient {
		label.SetText("Ready to install")
	} else {
		label.SetText("Select components to install")
	}
}

// getLocalIP returns the best non-loopback IPv4 address found on the local machine.
// It prioritizes physical network adapters over virtual ones and prefers:
// 1. 192.168.x.x addresses (typical home networks)
// 2. 10.x.x.x addresses
// 3. 172.16-31.x.x addresses
// Returns empty string if no suitable address is found.
func getLocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	type ipCandidate struct {
		ip        string
		priority  int // Lower is better: 1=192.168, 2=10.x, 3=172.16-31, 4=other private, 5=public
		isVirtual bool
	}

	var candidates []ipCandidate

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Skip known virtual adapters
		ifaceName := strings.ToLower(iface.Name)
		isVirtual := strings.Contains(ifaceName, "vethernet") ||
			strings.Contains(ifaceName, "hyper-v") ||
			strings.Contains(ifaceName, "wsl") ||
			strings.Contains(ifaceName, "tailscale") ||
			strings.Contains(ifaceName, "vmware") ||
			strings.Contains(ifaceName, "virtualbox") ||
			strings.Contains(ifaceName, "vbox") ||
			strings.Contains(ifaceName, "default switch") ||
			strings.HasPrefix(ifaceName, "vmnet") ||
			strings.HasPrefix(ifaceName, "virbr")

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Only consider IPv4 addresses
			if ip == nil || ip.To4() == nil {
				continue
			}

			ipStr := ip.String()
			priority := 5

			if ip.IsPrivate() {
				// Prioritize by IP range
				if strings.HasPrefix(ipStr, "192.168.") {
					priority = 1 // Best: typical home networks
				} else if strings.HasPrefix(ipStr, "10.") {
					priority = 2
				} else if strings.HasPrefix(ipStr, "172.") {
					// Check if it's in 172.16-31 range
					ipBytes := ip.To4()
					if ipBytes != nil && ipBytes[0] == 172 && ipBytes[1] >= 16 && ipBytes[1] <= 31 {
						priority = 3
					} else {
						priority = 4 // Other 172.x addresses
					}
				} else {
					priority = 4 // Other private addresses
				}
			}

			candidates = append(candidates, ipCandidate{
				ip:        ipStr,
				priority:  priority,
				isVirtual: isVirtual,
			})
		}
	}

	if len(candidates) == 0 {
		return ""
	}

	// Find best candidate: prefer non-virtual, then by priority
	best := candidates[0]
	for _, cand := range candidates[1:] {
		// If current best is virtual but candidate is not, prefer candidate
		if best.isVirtual && !cand.isVirtual {
			best = cand
			continue
		}
		// If both are virtual or both are physical, prefer by priority
		if best.isVirtual == cand.isVirtual {
			if cand.priority < best.priority {
				best = cand
			}
		}
		// If candidate is virtual but best is not, keep best
	}

	return best.ip
}

func runInstallation(installServer, installClient bool, serverDir, clientDir string,
	serverHost, serverPort, clientServer, clientPlayerName string,
	progress func(string), progressBar func(float64)) error {
	ghClient := installer.NewGitHubClient()
	downloader := installer.NewDownloader()

	progress("Fetching latest release from GitHub...")
	release, err := ghClient.GetLatestRelease()
	if err != nil {
		return fmt.Errorf("failed to fetch latest release: %w", err)
	}

	progress(fmt.Sprintf("Found release: %s", release.TagName))
	progressBar(0.1)

	if installServer {
		progress("Installing server...")
		if err := installComponent("server", serverDir, release, downloader, progress); err != nil {
			return fmt.Errorf("server installation failed: %w", err)
		}

		// Configure server settings
		progress("Configuring server...")
		if err := configureServer(serverDir, serverHost, serverPort); err != nil {
			progress(fmt.Sprintf("Warning: Failed to configure server: %v", err))
		}

		progressBar(0.5)
	}

	if installClient {
		progress("Installing client...")
		if err := installComponent("client", clientDir, release, downloader, progress); err != nil {
			return fmt.Errorf("client installation failed: %w", err)
		}
		progressBar(0.7)

		// Install dependencies (BizHawk and VC++ redistributable) for client
		progress("Installing dependencies...")
		bizhawkDir := filepath.Join(clientDir, "BizHawk")
		if err := os.MkdirAll(bizhawkDir, 0755); err != nil {
			return fmt.Errorf("failed to create BizHawk directory: %w", err)
		}

		// Use shared dependency manager
		depMgr := deps.NewDependencyManager(bizhawkDir, progress)
		bizhawkPath, err := depMgr.CheckAndInstallDependencies(nil) // No prompt needed in installer
		if err != nil {
			return fmt.Errorf("dependency installation failed: %w", err)
		}

		// Update bizhawkDir to match actual installation location
		bizhawkDir = filepath.Dir(bizhawkPath)

		// Copy server.lua to client directory (from extracted zip or current dir)
		serverLuaDest := filepath.Join(clientDir, "server.lua")
		serverLuaSrc := filepath.Join(clientDir, "server.lua") // May already be in zip
		if _, err := os.Stat(serverLuaSrc); os.IsNotExist(err) {
			serverLuaSrc = "server.lua" // Try current directory
		}
		if _, err := os.Stat(serverLuaSrc); err == nil {
			if data, err := os.ReadFile(serverLuaSrc); err == nil {
				if err := os.WriteFile(serverLuaDest, data, 0644); err == nil {
					progress("Copied server.lua to client directory")
				}
			}
		}

		// Configure bizhawk_path and other settings in client config.json
		progress("Configuring client...")
		if err := configureClient(clientDir, bizhawkDir, clientServer, clientPlayerName); err != nil {
			progress(fmt.Sprintf("Warning: Failed to configure client: %v", err))
		}

		progressBar(0.9)
	}

	progressBar(1.0)
	return nil
}

func installComponent(component, installDir string, release *installer.Release, downloader *installer.Downloader, progress func(string)) error {
	assetName := installer.GetAssetNameForPlatform(component)
	asset := release.FindAssetByName(assetName)
	if asset == nil {
		return fmt.Errorf("asset %s not found in release", assetName)
	}

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	progress(fmt.Sprintf("Downloading %s...", assetName))
	tempZip := filepath.Join(os.TempDir(), assetName)
	if err := downloader.DownloadFile(asset.DownloadURL, tempZip, nil); err != nil {
		return fmt.Errorf("failed to download %s: %w", assetName, err)
	}
	defer func() { _ = os.Remove(tempZip) }()

	progress(fmt.Sprintf("Extracting %s...", assetName))
	extractor := installer.NewBizHawkInstaller()
	if err := extractor.ExtractZip(tempZip, installDir); err != nil {
		return fmt.Errorf("failed to extract %s: %w", assetName, err)
	}

	progress(fmt.Sprintf("%s installed to %s", component, installDir))
	return nil
}

func configureServer(serverDir, host, port string) error {
	settingsPath := filepath.Join(serverDir, "state.json")

	settings := make(map[string]any)
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			// ignore invalid or missing settings file
			_ = err
		}
	}

	// Set host and port if provided
	if host != "" {
		settings["host"] = host
	}
	if port != "" {
		// Parse port as integer
		portInt, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("invalid port number: %w", err)
		}
		settings["port"] = portInt
	}

	// Only write if we have values to set
	if len(settings) > 0 {
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(settingsPath, data, 0644)
	}

	return nil
}

func configureClient(clientDir, bizhawkDir, serverURL, playerName string) error {
	configPath := filepath.Join(clientDir, "config.json")

	// Determine BizHawk executable path
	var bizhawkExe string
	if runtime.GOOS == "windows" {
		bizhawkExe = filepath.Join(bizhawkDir, "EmuHawk.exe")
	} else {
		bizhawkExe = filepath.Join(bizhawkDir, "EmuHawkMono.sh")
	}

	// Read existing config or create new
	cfg := make(map[string]string)
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			// ignore invalid or missing config
			_ = err
		}
	}

	// Set bizhawk_path
	cfg["bizhawk_path"] = bizhawkExe

	// Set server URL if provided
	if serverURL != "" {
		cfg["server"] = serverURL
	}

	// Set player name if provided
	if playerName != "" {
		cfg["name"] = playerName
	}

	// Write config
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// openFolder opens the specified folder in the default file manager
func openFolder(folderPath string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", folderPath)
	case "darwin":
		cmd = exec.Command("open", folderPath)
	default: // linux and others
		cmd = exec.Command("xdg-open", folderPath)
	}

	// Run in background and don't wait for completion
	go func() {
		_ = cmd.Start()
	}()
}

// getDefaultInstallDir returns an appropriate default installation directory
// avoiding system folders that might occur when running from extracted zip files
func getDefaultInstallDir() string {
	// Get current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		currentDir = "."
	}

	// Check if current directory is a system/temp directory
	if isSystemOrTempDir(currentDir) {
		// Try to get user directories in order of preference
		userDirs := []string{"Downloads", "Desktop", "Documents"}

		for _, dirName := range userDirs {
			if userDir := getUserDir(dirName); userDir != "" {
				return userDir
			}
		}

		// Fallback to home directory
		if homeDir, err := os.UserHomeDir(); err == nil {
			return homeDir
		}
	}

	return currentDir
}

// isSystemOrTempDir checks if the given path is a system or temporary directory
func isSystemOrTempDir(path string) bool {
	// Convert to lowercase for case-insensitive comparison on Windows
	lowerPath := strings.ToLower(path)

	// Check for common system directory names
	systemDirs := []string{
		"system32", "syswow64", "system", "windows", "winnt",
		"program files", "program files (x86)", "programdata",
		"temp", "tmp", "temporary", "cache", "appdata",
	}

	for _, sysDir := range systemDirs {
		if strings.Contains(lowerPath, sysDir) {
			return true
		}
	}

	// Check for paths that look like temp directories (containing random chars)
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts {
		// Skip empty parts
		if part == "" {
			continue
		}

		// Check if part looks like a temp directory name
		// (mostly numbers/letters, no spaces, reasonable length)
		if len(part) >= 8 && len(part) <= 16 {
			hasLetters := false
			hasNumbers := false
			hasSpaces := false

			for _, r := range part {
				if unicode.IsLetter(r) {
					hasLetters = true
				} else if unicode.IsDigit(r) {
					hasNumbers = true
				} else if r == ' ' {
					hasSpaces = true
					break
				}
			}

			// If it has both letters and numbers, no spaces, it's likely a temp dir
			if hasLetters && hasNumbers && !hasSpaces {
				return true
			}
		}
	}

	return false
}

// getUserDir returns the path to a user directory like Downloads, Desktop, etc.
func getUserDir(dirName string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Try platform-specific paths
	var possiblePaths []string

	switch runtime.GOOS {
	case "windows":
		// Windows: C:\Users\Username\Downloads
		possiblePaths = []string{filepath.Join(homeDir, dirName)}
	default:
		// Unix-like systems: /home/username/Downloads or /home/username/Desktop
		possiblePaths = []string{
			filepath.Join(homeDir, dirName),
			filepath.Join(homeDir, strings.ToLower(dirName)),
		}
	}

	// Check if any of the paths exist
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}
