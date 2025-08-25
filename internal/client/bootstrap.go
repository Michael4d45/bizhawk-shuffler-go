package client

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Bootstrap performs initial startup steps: parse flags, init logging,
// load or create config, prompt for missing values, persist defaults, and
// return commonly-used values for the caller.
func Bootstrap(args []string) (serverURL string, cfg Config, cfgFile string, logFile *os.File, httpClient *http.Client, err error) {
	var verbose bool
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	fs.StringVar(&serverURL, "server", "", "server URL (ws://, wss://, http:// or https://)")
	fs.BoolVar(&verbose, "v", false, "enable verbose logging to stdout and file")
	if err = fs.Parse(args); err != nil {
		return
	}

	logFile, err = InitLogging(verbose)
	if err != nil {
		err = fmt.Errorf("failed to init logging: %w", err)
		return
	}

	cfgFile = filepath.Join(".", "client_config.json")
	cfg, err = LoadConfig(cfgFile)
	if err != nil {
		err = fmt.Errorf("failed to load config: %w", err)
		return
	}
	// use the package logger
	// log.Printf("loaded config from %s", cfgFile) // callers may log

	// normalize stored server url
	cfg.NormalizeServer()

	reader := bufio.NewReader(os.Stdin)

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
		var u *url.URL
		u, err = url.Parse(serverURL)
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

	if jb, _ := json.MarshalIndent(cfg, "", "  "); jb != nil {
		_ = os.WriteFile(cfgFile, jb, 0644)
		// log.Printf("wrote initial config to %s", cfgFile)
	}

	if err = cfg.EnsureDefaults(); err != nil {
		err = fmt.Errorf("EnsureDefaults: %w", err)
		return
	}
	// log.Printf("persisted default config to %s", cfgFile)
	_ = cfg.Save(cfgFile)

	httpClient = &http.Client{Timeout: 0}
	return
}
