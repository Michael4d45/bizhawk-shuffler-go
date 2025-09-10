package client

import (
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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// ErrNotFound is returned when a requested remote save/file is not present on the server
var ErrNotFound = errors.New("not found")

// Client represents a running client instance and holds its dependencies
// and runtime state.
type Client struct {
	cfg     Config
	logFile *os.File

	wsClient     *WSClient
	api          *API
	bhController *BizHawkController
	bipc         *BizhawkIPC
	// TODO: Add discovery listener field
	discoveryListener *DiscoveryListener
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
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	fs.StringVar(&serverFlag, "server", "", "server URL (ws://, wss://, http:// or https://)")
	fs.BoolVar(&verbose, "v", false, "enable verbose logging to stdout and file")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	logFile, err := InitLogging(verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to init logging: %w", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	reader := bufio.NewReader(os.Stdin)
	serverURL := serverFlag
	for serverURL == "" {
		if s, ok := cfg["server"]; ok && s != "" {
			serverURL = s
			break
		}

		// Try discovery first
		fmt.Println("Attempting to discover servers on the network...")
		startTime := time.Now()
		discoveredURL, err := discoverServerWithPrompt(cfg)
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
				cfg["server"] = u.String()
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
		cfg["server"] = u.String()
	}

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
		cfg["name"] = name
		break
	}

	_ = cfg.Save()

	if err := cfg.EnsureDefaults(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("EnsureDefaults: %w", err)
	}
	_ = cfg.Save()

	httpClient := &http.Client{Timeout: 0}

	wsURL, serverHTTP, err := BuildWSAndHTTP(serverURL, cfg)
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}

	api := NewAPI(serverHTTP, httpClient, cfg)

	bipc := NewBizhawkIPC("127.0.0.1", 55355)

	// create a controller with a temporary API (no base URL) to perform any
	// installation/download steps before the real server API is known.
	bhController := NewBizHawkController(api, httpClient, cfg, bipc)
	if err := bhController.EnsureBizHawkInstalled(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("EnsureBizHawkInstalled: %w", err)
	}
	_ = cfg.Save()
	// update controller to use the real API
	bhController.api = api
	wsClient := NewWSClient(wsURL, api, bipc)

	c := &Client{
		cfg:          cfg,
		logFile:      logFile,
		wsClient:     wsClient,
		api:          api,
		bhController: bhController,
		bipc:         bipc,
		// TODO: Initialize discovery listener
		discoveryListener: nil,
	}

	return c, nil
}

// Run starts the client's runtime: opens connections, starts goroutines and
// blocks until shutdown completes.
func (c *Client) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		}
	}()

	log.Printf("[Client] starting bizhawk ipc")
	if err := c.bipc.Start(ctx); err != nil {
		log.Printf("bizhawk ipc start error: %v", err)
	} else {
		log.Printf("bizhawk ipc started")
	}
	defer func() {
		_ = c.bipc.Close()
		log.Printf("closing bizhawk ipc")
	}()

	log.Printf("[Client] starting bizhawk ipc")
	c.bhController.StartIPCGoroutine(ctx)

	log.Printf("[Client] starting websocket client")
	c.wsClient.Start(ctx, c.cfg)
	defer c.wsClient.Stop()

	<-ctx.Done()
	log.Printf("[Client] shutdown complete")
}

// InitLogging sets up global logging and returns the opened log file which the
// caller should Close when finished.
func InitLogging(verbose bool) (*os.File, error) {
	f, err := os.OpenFile("client.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		return nil, err
	}
	if verbose {
		mw := io.MultiWriter(os.Stdout, f)
		log.SetOutput(mw)
	} else {
		log.SetOutput(f)
	}
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	return f, nil
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

// TODO: Add methods for managing discovery listener
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
