package client

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/michael4d45/bizshuffle/internal/deps"
	"github.com/michael4d45/bizshuffle/internal/types"
)

// ErrNotFound is returned when a requested remote save/file is not present on the server
var ErrNotFound = errors.New("not found")

// ErrFileLocked is returned when a save file cannot be accessed because it's in use by another process
var ErrFileLocked = errors.New("file locked by another process")

// Client represents a running client instance and holds its dependencies
// and runtime state.
type Client struct {
	cfg     Config
	logFile *os.File

	wsClient     *WSClient
	api          *API
	bhController *BizHawkController
	bipc         *BizhawkIPC

	discoveryListener *DiscoveryListener
	pluginSyncManager *PluginSyncManager
}

// New creates and initializes a Client from CLI args.
func New(args []string) (*Client, error) {
	// ensure ./roms and ./saves dirs exist
	if err := os.MkdirAll("./roms", 0755); err != nil {
		return nil, fmt.Errorf("failed to create roms directory: %w", err)
	}
	if err := os.MkdirAll("./saves", 0755); err != nil {
		return nil, fmt.Errorf("failed to create saves directory: %w", err)
	}

	var verbose bool
	var serverFlag string
	var noGui bool
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	fs.StringVar(&serverFlag, "server", "", "server URL (ws://, wss://, http:// or https://)")
	fs.BoolVar(&verbose, "v", false, "enable verbose logging to stdout and file")
	fs.BoolVar(&noGui, "no-gui", false, "disable graphical user interface")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	c := &Client{}
	var err error
	c.cfg, err = LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	c.cfg["no_gui"] = strconv.FormatBool(noGui)

	logFile, err := InitLogging(verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to init logging: %w", err)
	}
	c.logFile = logFile

	if err := c.cfg.EnsureDefaults(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("EnsureDefaults: %w", err)
	}

	httpClient := &http.Client{Timeout: 0}
	c.api = NewAPI("", httpClient, c.cfg) // Temporary API without server URL

	// Check and install dependencies if needed
	// Determine install directory - default to installing next to the client executable
	bizhawkDir := ""
	if exe, err2 := os.Executable(); err2 == nil {
		bizhawkDir = filepath.Join(filepath.Dir(exe), "BizHawk")
	} else if cwd, err2 := os.Getwd(); err2 == nil {
		bizhawkDir = filepath.Join(cwd, "BizHawk")
	}

	configuredPath := c.cfg["bizhawk_path"]
	// If bizhawk_path is configured, use its directory as the install directory
	if configuredPath != "" {
		bizhawkDir = filepath.Dir(configuredPath)
	}

	// Create dependency manager and check/install dependencies
	progressCallback := func(msg string) {
		fmt.Fprintf(os.Stderr, "%s\n", msg)
		log.Printf("Dependency install: %s", msg)
	}

	promptCallback := func(dependencyName string) bool {
		fmt.Fprintf(os.Stderr, "\n%s is not installed. Would you like to install it now? (y/n): ", dependencyName)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		return response == "y" || response == "yes"
	}

	depMgr := deps.NewDependencyManagerWithPath(bizhawkDir, configuredPath, progressCallback)
	resolvedBizhawkPath, err := depMgr.CheckAndInstallDependencies(promptCallback)
	if err != nil {
		_ = logFile.Close()
		fmt.Fprintf(os.Stderr, "ERROR: Dependency installation failed: %v\n", err)
		log.Printf("Dependency installation failed: %v", err)
		os.Exit(1)
	}

	// Update config with resolved BizHawk path if it changed
	oldPath := c.cfg["bizhawk_path"]
	if resolvedBizhawkPath != oldPath {
		c.cfg["bizhawk_path"] = resolvedBizhawkPath
		log.Printf("Updated bizhawk_path from %s to %s", oldPath, resolvedBizhawkPath)
		if err := c.cfg.Save(); err != nil {
			log.Printf("Warning: Failed to save updated bizhawk_path to config: %v", err)
		}
	}

	bhController := NewBizHawkController(nil, httpClient, c.cfg, nil, nil)
	bhController.initialized = true

	serverURL := serverFlag
	isGui := !noGui

	if !isGui {
		reader := bufio.NewReader(os.Stdin)
		for serverURL == "" {
			if s, ok := c.cfg["server"]; ok && s != "" {
				serverURL = s
				break
			}

			// Try discovery first
			fmt.Println("Attempting to discover servers on the network...")
			startTime := time.Now()
			discoveredURL, err := discoverServerWithPrompt(c.cfg)
			discoveryDuration := time.Since(startTime)
			log.Printf("[Client] Discovery completed in %v", discoveryDuration)
			if err != nil {
				log.Printf("Discovery failed: %v", err)
				fmt.Println("Falling back to manual entry...")
			} else if discoveredURL != "" {
				// Validate and save discovered URL just like manual entry
				u, err := url.Parse(discoveredURL)
				if err != nil {
					fmt.Printf("invalid discovered server URL: %v\n", err)
					fmt.Println("Falling back to manual entry...")
				} else {
					switch u.Scheme {
					case "ws":
						u.Scheme = "http"
					case "wss":
						u.Scheme = "https"
					}
					u.Path = ""
					u.RawQuery = ""
					u.Fragment = ""
					c.cfg["server"] = u.String()
					serverURL = u.String()
					break
				}
			}

			// Fallback to manual entry
			fmt.Print("Server URL (ws://host:port/ws or http://host:port): ")
			line, _ := reader.ReadString('\n')
			serverURL = strings.TrimSpace(line)
			if serverURL == "" {
				fmt.Println("server URL cannot be empty")
				continue
			}
			if strings.HasPrefix(serverURL, "ws://") || strings.HasPrefix(serverURL, "wss://") || strings.HasPrefix(serverURL, "http://") || strings.HasPrefix(serverURL, "https://") {
				// ok
			} else {
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
			c.cfg["server"] = u.String()
		}

		for {
			if n, ok := c.cfg["name"]; ok && strings.TrimSpace(n) != "" {
				break
			}
			fmt.Print("Player name: ")
			line, _ := reader.ReadString('\n')
			name := strings.TrimSpace(line)
			if name == "" {
				fmt.Println("player name cannot be empty")
				continue
			}
			c.cfg["name"] = name
			break
		}
	} else {
		if serverURL == "" {
			serverURL = c.cfg["server"]
		}
	}

	_ = c.cfg.Save()

	var wsURL string
	var serverHTTP string
	if serverURL != "" {
		wsURL, serverHTTP, err = BuildWSAndHTTP(serverURL, c.cfg)
		if err != nil {
			_ = logFile.Close()
			return nil, err
		}
	}

	c.api = NewAPI(serverHTTP, httpClient, c.cfg)

	bipc := NewBizhawkIPC()

	wsClient := NewWSClient(wsURL, c.api, bipc)

	bhController.wsClient = wsClient
	bhController.api = c.api
	bhController.bipc = bipc

	// Ensure BizhawkFiles are downloaded if needed (check for config.ini)
	if err := bhController.EnsureBizhawkFiles(); err != nil {
		log.Printf("Warning: Failed to ensure BizhawkFiles: %v", err)
		// Don't fail startup, just log a warning
	}

	_ = c.cfg.Save()

	// Initialize plugin sync manager
	pluginSyncManager := NewPluginSyncManager(c.api, httpClient, c.cfg)

	c.wsClient = wsClient
	c.bhController = bhController
	c.bipc = bipc
	c.discoveryListener = nil
	c.pluginSyncManager = pluginSyncManager

	return c, nil
}

// GetConfig returns the client's configuration.
func (c *Client) GetConfig() Config {
	return c.cfg
}

// RunGUI starts the client with a graphical user interface.
func (c *Client) RunGUI() {
	// Recover from panics to ensure we log something
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Client] PANIC in RunGUI(): %v", r)
			panic(r) // Re-panic so main() can catch it
		}
	}()

	log.Printf("[Client] RunGUI() starting")
	fmt.Fprintf(os.Stderr, "[Client] RunGUI() starting\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		log.Printf("[Client] cancelling context and cleaning up")
		cancel()
	}()

	// Set up signal handling as a backup to BizHawkController's signal handling
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Printf("[Client] received signal %v, initiating shutdown", sig)
		cancel()
	}()

	// Launch BizHawk if auto_open_bizhawk is enabled
	if c.cfg.GetBool("auto_open_bizhawk") {
		go func() {
			log.Printf("[Client] auto-launching BizHawk")
			if err := c.bhController.LaunchAndManage(ctx, cancel); err != nil {
				log.Printf("LaunchAndManage error: %v", err)
				cancel() // Ensure we exit if LaunchAndManage fails
			}
		}()
	}

	log.Printf("[Client] starting bizhawk ipc")
	if err := c.bipc.Start(ctx); err != nil {
		log.Printf("bizhawk ipc start error: %v", err)
		log.Printf("[Client] bizhawk ipc failed to start, continuing anyway...")
	} else {
		log.Printf("bizhawk ipc started")
	}
	defer func() {
		_ = c.bipc.Close()
		log.Printf("closing bizhawk ipc")
	}()

	log.Printf("[Client] starting bizhawk ipc goroutine")
	c.bhController.StartIPCGoroutine(ctx)

	log.Printf("[Client] starting websocket client")
	c.wsClient.Start(ctx, c.cfg)
	defer c.wsClient.Stop()

	// Perform initial plugin sync
	log.Printf("[Client] performing initial plugin sync")
	if result, err := c.pluginSyncManager.SyncPlugins(); err != nil {
		log.Printf("[Client] plugin sync failed: %v", err)
	} else {
		log.Printf("[Client] plugin sync completed: %d total, %d downloaded, %d updated, %d removed in %v",
			result.TotalPlugins, result.Downloaded, result.Updated, result.Removed, result.Duration)
		if len(result.Errors) > 0 {
			log.Printf("[Client] plugin sync had %d errors:", len(result.Errors))
			for _, err := range result.Errors {
				log.Printf("[Client]   - %s", err)
			}
		}
	}

	// Start the GUI (this blocks until the window is closed)
	gui := NewGUI(c, ctx, cancel)
	gui.Show()

	log.Printf("[Client] GUI window closed, shutdown complete")
}

// Run starts the client's runtime: opens connections, starts goroutines and
// blocks until shutdown completes.
func (c *Client) Run() {
	// Recover from panics to ensure we log something
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Client] PANIC in Run(): %v", r)
			panic(r) // Re-panic so main() can catch it
		}
	}()

	log.Printf("[Client] Run() starting")
	fmt.Fprintf(os.Stderr, "[Client] Run() starting\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		log.Printf("[Client] cancelling context and cleaning up")
		cancel()
	}()

	// Set up signal handling as a backup to BizHawkController's signal handling
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Printf("[Client] received signal %v, initiating shutdown", sig)
		cancel()
	}()

	go func() {
		log.Printf("[Client] client starting")
		if err := c.bhController.LaunchAndManage(ctx, cancel); err != nil {
			log.Printf("LaunchAndManage error: %v", err)
			cancel() // Ensure we exit if LaunchAndManage fails
		}
	}()

	log.Printf("[Client] starting bizhawk ipc")
	if err := c.bipc.Start(ctx); err != nil {
		log.Printf("bizhawk ipc start error: %v", err)
		log.Printf("[Client] bizhawk ipc failed to start, continuing anyway...")
	} else {
		log.Printf("bizhawk ipc started")
	}
	defer func() {
		_ = c.bipc.Close()
		log.Printf("closing bizhawk ipc")
	}()

	log.Printf("[Client] starting bizhawk ipc goroutine")
	c.bhController.StartIPCGoroutine(ctx)

	log.Printf("[Client] starting websocket client")
	c.wsClient.Start(ctx, c.cfg)
	defer c.wsClient.Stop()

	// Perform initial plugin sync
	log.Printf("[Client] performing initial plugin sync")
	if result, err := c.pluginSyncManager.SyncPlugins(); err != nil {
		log.Printf("[Client] plugin sync failed: %v", err)
	} else {
		log.Printf("[Client] plugin sync completed: %d total, %d downloaded, %d updated, %d removed in %v",
			result.TotalPlugins, result.Downloaded, result.Updated, result.Removed, result.Duration)
		if len(result.Errors) > 0 {
			log.Printf("[Client] plugin sync had %d errors:", len(result.Errors))
			for _, err := range result.Errors {
				log.Printf("[Client]   - %s", err)
			}
		}
	}

	log.Printf("[Client] entering main event loop, waiting for shutdown signal...")
	<-ctx.Done()
	log.Printf("[Client] context cancelled, shutdown complete")
}

// InitLogging sets up global logging and returns the opened log file which the
// caller should Close when finished. It rotates old log files by zipping them
// with a timestamp, similar to Minecraft's logging system.
func InitLogging(verbose bool) (*os.File, error) {
	// Create logs directory if it doesn't exist
	logsDir := "logs"
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to create logs directory: %v\n", err)
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logFilePath := filepath.Join(logsDir, "client.log")

	// Check if old log file exists in logs directory and zip it
	oldLogPath := filepath.Join(logsDir, "client.log")
	if _, err := os.Stat(oldLogPath); err == nil {
		// Old log file exists, zip it with timestamp
		timestamp := time.Now().Format("2006-01-02-15-04-05")
		zipFileName := fmt.Sprintf("client-%s.log.zip", timestamp)
		zipFilePath := filepath.Join(logsDir, zipFileName)

		if err := zipLogFile(oldLogPath, zipFilePath); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: Failed to zip old log file: %v\n", err)
			// Continue anyway, don't fail startup
		} else {
			fmt.Fprintf(os.Stderr, "Zipped old log file to: %s\n", zipFilePath)
			// Remove the old log file after successful zipping
			if err := os.Remove(oldLogPath); err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: Failed to remove old log file: %v\n", err)
			}
		}
	}

	// Create new log file
	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to open log file '%s': %v\n", logFilePath, err)
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	if verbose {
		mw := io.MultiWriter(os.Stdout, f)
		log.SetOutput(mw)
	} else {
		log.SetOutput(f)
	}
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("[InitLogging] Logging initialized (verbose=%v, logs_dir=%s)", verbose, logsDir)
	return f, nil
}

// zipLogFile creates a zip archive of the specified log file
func zipLogFile(logFilePath, zipFilePath string) error {
	// Open the log file for reading
	logFile, err := os.Open(logFilePath)
	if err != nil {
		return fmt.Errorf("failed to open log file for zipping: %w", err)
	}
	defer logFile.Close()

	// Create the zip file
	zipFile, err := os.Create(zipFilePath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	// Create zip writer
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Get the filename for the zip entry
	_, fileName := filepath.Split(logFilePath)

	// Create zip entry
	zipEntry, err := zipWriter.Create(fileName)
	if err != nil {
		return fmt.Errorf("failed to create zip entry: %w", err)
	}

	// Copy log file content to zip entry
	_, err = io.Copy(zipEntry, logFile)
	if err != nil {
		return fmt.Errorf("failed to copy log content to zip: %w", err)
	}

	return nil
}

// BuildWSAndHTTP takes the -server flag value (which may be a ws:// or http://
// form) and the stored config and returns a websocket URL to connect to and a
// corresponding http(s) base URL for REST calls. It mirrors the URL logic
// previously in run.go so the construction can be reused and tested.
func BuildWSAndHTTP(serverFlag string, cfg Config) (wsURL string, serverHTTP string, err error) {
	serverHTTP = ""
	if s, ok := cfg["server"]; ok && s != "" {
		serverHTTP = s
	}

	if strings.HasPrefix(serverFlag, "ws://") || strings.HasPrefix(serverFlag, "wss://") {
		u, err := url.Parse(serverFlag)
		if err != nil {
			return "", "", fmt.Errorf("invalid server url %q: %w", serverFlag, err)
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
		return wsURL, serverHTTP, nil
	}

	if serverHTTP == "" {
		return "", "", fmt.Errorf("no server configured for websocket and -server flag not provided")
	}
	hu, err := url.Parse(serverHTTP)
	if err != nil {
		return "", "", fmt.Errorf("invalid configured server %q: %w", serverHTTP, err)
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
	return wsURL, serverHTTP, nil
}

// MonitorProcess starts a goroutine that waits for the process to exit and
// calls onExit(err). onExit will be called regardless of whether the process
// exited successfully or with an error.
func MonitorProcess(cmd *exec.Cmd, onExit func(error)) {
	if cmd == nil {
		go onExit(fmt.Errorf("nil cmd"))
		return
	}
	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Printf("BizHawk exited with error: %v", err)
		} else {
			log.Printf("BizHawk exited")
		}
		onExit(err)
	}()
}

// StartDiscovery initializes and starts the discovery listener
func (c *Client) StartDiscovery(ctx context.Context) error {
	if c.discoveryListener != nil {
		return c.discoveryListener.Start(ctx)
	}

	// Create default discovery config
	config := types.GetDefaultDiscoveryConfig()

	// Initialize listener
	c.discoveryListener = NewDiscoveryListener(config)
	return c.discoveryListener.Start(ctx)
}

// StopDiscovery stops the discovery listener
func (c *Client) StopDiscovery() error {
	if c.discoveryListener != nil {
		return c.discoveryListener.Stop()
	}
	return nil
}

// PromptForServerWithDiscovery shows server selection menu with discovered servers
func (c *Client) PromptForServerWithDiscovery() (string, error) {
	reader := bufio.NewReader(os.Stdin)

	// Parse timeout from config
	timeoutStr := c.cfg["discovery_timeout_seconds"]
	timeoutSeconds := 10 // default
	if timeoutStr != "" {
		if parsed, err := strconv.Atoi(timeoutStr); err == nil && parsed > 0 {
			timeoutSeconds = parsed
		}
	}

	// Start discovery in background
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	if err := c.StartDiscovery(ctx); err != nil {
		log.Printf("Failed to start discovery: %v", err)
	}

	// Wait a bit for servers to be discovered
	time.Sleep(2 * time.Second)

	// Get discovered servers
	servers := c.discoveryListener.GetDiscoveredServers()

	// Stop discovery
	if err := c.StopDiscovery(); err != nil {
		log.Printf("Error stopping discovery: %v", err)
	}

	if len(servers) == 0 {
		fmt.Println("No servers discovered on the network.")
	} else {
		fmt.Println("Discovered servers:")
		for i, server := range servers {
			fmt.Printf("%d. %s (%s:%d)\n", i+1, server.Name, server.Host, server.Port)
		}
		fmt.Println("0. Enter server manually")
	}

	fmt.Print("Select server (number or 0 for manual): ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "0" || len(servers) == 0 {
		// Manual entry
		return promptForServerManually(reader)
	}

	// Parse selection
	index := 0
	if _, err := fmt.Sscanf(input, "%d", &index); err != nil || index < 1 || index > len(servers) {
		fmt.Println("Invalid selection, using manual entry.")
		return promptForServerManually(reader)
	}

	selectedServer := servers[index-1]
	return selectedServer.GetServerURL(), nil
}

// discoverServerWithPrompt performs server discovery and prompts user for selection
func discoverServerWithPrompt(cfg Config) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	// Parse timeout from config
	timeoutStr := cfg["discovery_timeout_seconds"]
	timeoutSeconds := 10 // default
	if timeoutStr != "" {
		if parsed, err := strconv.Atoi(timeoutStr); err == nil && parsed > 0 {
			timeoutSeconds = parsed
		}
	}

	// Create discovery config
	config := types.GetDefaultDiscoveryConfig()

	// Create discovery listener
	listener := NewDiscoveryListener(config)

	// Start discovery with context timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	if err := listener.Start(ctx); err != nil {
		return "", fmt.Errorf("failed to start discovery: %w", err)
	}

	// Wait for context to finish (timeout or cancel)
	<-ctx.Done()

	// Get discovered servers after timeout
	servers := listener.GetDiscoveredServers()

	// Stop discovery (should be a no-op if already stopped)
	if err := listener.Stop(); err != nil {
		log.Printf("Error stopping discovery: %v", err)
	}

	if len(servers) == 0 {
		fmt.Println("No servers discovered on the network.")
		return "", nil // Return empty string to trigger manual entry
	}

	fmt.Println("Discovered servers:")
	for i, server := range servers {
		fmt.Printf("%d. %s (%s:%d)\n", i+1, server.Name, server.Host, server.Port)
	}
	fmt.Println("0. Enter server manually")

	fmt.Print("Select server (number or 0 for manual): ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "0" {
		return "", nil // User chose manual entry
	}

	// Parse selection
	index := 0
	if _, err := fmt.Sscanf(input, "%d", &index); err != nil || index < 1 || index > len(servers) {
		fmt.Println("Invalid selection, using manual entry.")
		return "", nil
	}

	selectedServer := servers[index-1]
	return selectedServer.GetServerURL(), nil
}

// promptForServerManually prompts the user to manually enter a server URL
func promptForServerManually(reader *bufio.Reader) (string, error) {
	for {
		fmt.Print("Server URL (ws://host:port/ws or http://host:port): ")
		line, _ := reader.ReadString('\n')
		serverURL := strings.TrimSpace(line)
		if serverURL == "" {
			fmt.Println("server URL cannot be empty")
			continue
		}
		if strings.HasPrefix(serverURL, "ws://") || strings.HasPrefix(serverURL, "wss://") || strings.HasPrefix(serverURL, "http://") || strings.HasPrefix(serverURL, "https://") {
			// ok
		} else {
			fmt.Println("server URL must start with ws://, wss://, http:// or https://")
			continue
		}
		u, err := url.Parse(serverURL)
		if err != nil {
			fmt.Printf("invalid server URL: %v\n", err)
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
		return u.String(), nil
	}
}
