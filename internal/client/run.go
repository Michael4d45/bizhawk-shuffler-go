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
	"strings"
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
}

// New creates and initializes a Client from CLI args.
func New(args []string) (*Client, error) {
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

	bipc := NewBizhawkIPC("127.0.0.1", 55355)

	// create a controller with a temporary API (no base URL) to perform any
	// installation/download steps before the real server API is known.
	installAPI := NewAPI("", httpClient, cfg)
	bhController := NewBizHawkController(installAPI, httpClient, cfg, bipc)
	if err := bhController.EnsureBizHawkInstalled(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("EnsureBizHawkInstalled: %w", err)
	}
	_ = cfg.Save()

	wsURL, serverHTTP, err := BuildWSAndHTTP(serverURL, cfg)
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}

	api := NewAPI(serverHTTP, httpClient, cfg)
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
	}

	return c, nil
}

// Run starts the client's runtime: opens connections, starts goroutines and
// blocks until shutdown completes.
func (c *Client) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
