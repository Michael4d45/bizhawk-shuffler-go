package client

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/michael4d45/bizshuffle/internal/types"
)

// Controller wires dependencies and handles incoming commands.
type Controller struct {
	cfg              Config
	bipc             *BizhawkIPC
	api              *API
	progressTracking *ProgressTrackingAPI
	writeJSON        func(types.Command) error
	// mainGames caches the server's main games list for extra_files lookup
	mainGames []types.GameEntry
	mu        sync.RWMutex // protects mainGames and state fields

	// state fields
	currentGame       string
	currentInstanceID string
	pendingFile       string

	// helloAck signals when hello has been acknowledged (first CmdGamesUpdate received)
	helloAck chan struct{}

	// restartBizhawk is called to restart BizHawk after config updates
	restartBizhawk func()
	// closeBizhawk is called to close BizHawk
	closeBizhawk func()
	// terminateBizhawkForConfig is called to terminate BizHawk for config updates (without cancelling client context)
	terminateBizhawkForConfig func()
	// launchBizhawk is called to launch BizHawk (normal launch, resets restart mode)
	launchBizhawk func()
	// launchBizhawkForConfig is called to launch BizHawk after config update (preserves restart mode)
	launchBizhawkForConfig func()
	// setRestartMode is called to set BizHawk restart mode
	setRestartMode func(bool)
}

func NewController(cfg Config, bipc *BizhawkIPC, api *API, writeJSON func(types.Command) error) *Controller {
	return NewControllerWithHelloAckAndCallbacks(cfg, bipc, api, writeJSON, nil, nil, nil, nil, nil, nil, nil)
}

func NewControllerWithHelloAck(cfg Config, bipc *BizhawkIPC, api *API, writeJSON func(types.Command) error, helloAck chan struct{}) *Controller {
	return NewControllerWithHelloAckAndCallbacks(cfg, bipc, api, writeJSON, helloAck, nil, nil, nil, nil, nil, nil)
}

func NewControllerWithHelloAckAndRestart(cfg Config, bipc *BizhawkIPC, api *API, writeJSON func(types.Command) error, helloAck chan struct{}, restartBizhawk func()) *Controller {
	return NewControllerWithHelloAckAndCallbacks(cfg, bipc, api, writeJSON, helloAck, restartBizhawk, nil, nil, nil, nil, nil)
}

func NewControllerWithHelloAckAndCallbacks(cfg Config, bipc *BizhawkIPC, api *API, writeJSON func(types.Command) error, helloAck chan struct{}, restartBizhawk func(), closeBizhawk func(), terminateBizhawkForConfig func(), launchBizhawk func(), launchBizhawkForConfig func(), setRestartMode func(bool)) *Controller {
	c := &Controller{
		cfg:                       cfg,
		bipc:                      bipc,
		api:                       api,
		writeJSON:                 writeJSON,
		mainGames:                 make([]types.GameEntry, 0),
		helloAck:                  helloAck,
		restartBizhawk:            restartBizhawk,
		closeBizhawk:              closeBizhawk,
		terminateBizhawkForConfig: terminateBizhawkForConfig,
		launchBizhawk:             launchBizhawk,
		launchBizhawkForConfig:    launchBizhawkForConfig,
		setRestartMode:            setRestartMode,
	}
	c.progressTracking = NewProgressTrackingAPI(api, c)
	return c
}

// SetRestartBizhawkCallback sets the callback function to restart BizHawk
func (c *Controller) SetRestartBizhawkCallback(restartFunc func()) {
	c.restartBizhawk = restartFunc
}

// SetBizhawkCallbacks sets the callback functions for BizHawk control
func (c *Controller) SetBizhawkCallbacks(closeFunc func(), terminateForConfigFunc func(), launchFunc func(), launchForConfigFunc func(), setRestartModeFunc func(bool)) {
	c.closeBizhawk = closeFunc
	c.terminateBizhawkForConfig = terminateForConfigFunc
	c.launchBizhawk = launchFunc
	c.launchBizhawkForConfig = launchForConfigFunc
	c.setRestartMode = setRestartModeFunc
}

// Handle processes a single incoming command. It launches goroutines for
// commands that should run asynchronously (keeps original behavior).
func (c *Controller) Handle(ctx context.Context, cmd types.Command) {
	sendAck := func(id string) { _ = c.writeJSON(types.Command{Cmd: types.CmdAck, ID: id}) }
	sendNack := func(id, reason string) {
		_ = c.writeJSON(types.Command{Cmd: types.CmdNack, ID: id, Payload: map[string]string{"reason": reason}})
	}

	switch cmd.Cmd {
	case types.CmdResume:
		go func(id string) {
			ctx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
			defer cancel2()
			if err := c.bipc.SendResume(ctx2); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdPause:
		go func(id string) {
			if err := c.bipc.SendPause(ctx); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdSwap:
		go func(id string) {
			game := ""
			instanceID := ""
			if m, ok := cmd.Payload.(map[string]any); ok {
				if g, ok := m["game"].(string); ok {
					game = g
				}
				if iid, ok := m["instance_id"].(string); ok {
					instanceID = iid
				}
			}

			c.mu.Lock()
			oldInstanceID := c.currentInstanceID
			c.currentGame = game
			c.currentInstanceID = instanceID
			c.mu.Unlock()

			// Disable auto-save during swap to prevent race conditions
			if err := c.bipc.SendAutoSaveDisable(ctx); err != nil {
				log.Printf("Failed to disable auto-save: %v", err)
			}
			// Re-enable auto-save when done (success or failure)
			defer func() {
				if err := c.bipc.SendAutoSaveEnable(ctx); err != nil {
					log.Printf("Failed to re-enable auto-save: %v", err)
				}
			}()

			if game != "" {
				c.mu.Lock()
				c.pendingFile = game
				c.mu.Unlock()

				ctx2, cancel2 := context.WithTimeout(ctx, 30*time.Second)
				log.Printf("ensuring ROM present for game=%s", game)
				if err := c.progressTracking.EnsureFileWithProgress(ctx2, game); err != nil {
					cancel2()
					c.mu.Lock()
					c.pendingFile = ""
					c.mu.Unlock()
					sendNack(id, "download failed: "+err.Error())
					return
				}
				cancel2()

				c.mu.Lock()
				c.pendingFile = ""
				c.mu.Unlock()
			}
			log.Printf("sending swap to lua for game=%s", game)

			if c.bipc.IsReady() {
				_ = c.bipc.SendSave(ctx)
				if err := c.verifySaveWithRetry(c.bipc.instanceID); err != nil {
					sendNack(id, "save verification failed: "+err.Error())
					return
				}
			}
			if err := c.EnsureSaveState(oldInstanceID, instanceID); err != nil {
				sendNack(id, "save state orchestration failed: "+err.Error())
				return
			}

			if err := c.bipc.SendSwap(ctx, game, instanceID); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdClearSaves:
		go func(id string) {
			c.ClearSaves()
			if err := c.bipc.SendRestart(ctx); err != nil {
				sendNack(id, err.Error())
				return
			}

			if err := c.bipc.SendMessage(ctx, "clear_saves"); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdGamesUpdate:
		// Signal hello acknowledgment on first games update
		if c.helloAck != nil {
			select {
			case c.helloAck <- struct{}{}:
			default:
			}
			c.helloAck = nil // prevent multiple signals
		}

		go func(payload any) {
			required := make(map[string]struct{})
			// Build set of instance games we need
			games := make(map[string]struct{})
			var mainGames []types.GameEntry

			if m, ok := payload.(map[string]any); ok {
				// Parse and cache main_games first
				if mg, ok := m["main_games"].([]any); ok {
					for _, mei := range mg {
						if em, ok := mei.(map[string]any); ok {
							var entry types.GameEntry
							if f, ok := em["file"].(string); ok {
								entry.File = f
							}
							if extras, ok := em["extra_files"].([]any); ok {
								for _, ex := range extras {
									if exs, ok := ex.(string); ok {
										entry.ExtraFiles = append(entry.ExtraFiles, exs)
									}
								}
							}
							if entry.File != "" {
								mainGames = append(mainGames, entry)
							}
						}
					}
				}
				// Update the cached main games
				c.SetMainGames(mainGames)

				if gis, ok := m["game_instances"].([]any); ok {
					for _, gi := range gis {
						if im, ok := gi.(map[string]any); ok {
							if g, ok2 := im["game"].(string); ok2 && g != "" {
								games[g] = struct{}{}
								required[g] = struct{}{}
							}
						}
					}
				}
				if gg, ok := m["games"].([]any); ok {
					for _, gi := range gg {
						if g, ok := gi.(string); ok {
							games[g] = struct{}{}
							required[g] = struct{}{}
						}
					}
				}
				// extras from main_games when primary is in instanceGames
				for _, entry := range mainGames {
					if _, isActive := games[entry.File]; isActive {
						for _, extra := range entry.ExtraFiles {
							required[extra] = struct{}{}
						}
					}
				}
			}
			var wg sync.WaitGroup
			errCh := make(chan error, 8)
			for name := range required {
				n := name
				wg.Add(1)
				go func(fname string) {
					defer wg.Done()
					ctx2, cancel2 := context.WithTimeout(ctx, 60*time.Second)
					defer cancel2()
					if err := c.progressTracking.EnsureFileWithProgress(ctx2, fname); err != nil {
						errCh <- fmt.Errorf("failed to download %s: %w", fname, err)
						return
					}
					log.Printf("games_update: ensured file %s", fname)
				}(n)
			}
			wg.Wait()
			close(errCh)
			errList := []string{}
			for e := range errCh {
				log.Printf("games_update error: %v", e)
				errList = append(errList, e.Error())
			}
			hasFiles := len(errList) == 0
			ackPayload := map[string]any{"has_files": hasFiles}
			if !hasFiles {
				ackPayload["errors"] = errList
			}
			_ = c.writeJSON(types.Command{Cmd: types.CmdGamesUpdateAck, ID: fmt.Sprintf("%d", time.Now().UnixNano()), Payload: ackPayload})
		}(cmd.Payload)
	case types.CmdMessage:
		go func(id string) {
			message := ""
			duration := 3.0
			x := 10
			y := 10
			fontsize := 12
			fg := "#FFFFFF"
			bg := "#000000"

			if m, ok := cmd.Payload.(map[string]any); ok {
				if msg, ok := m["message"].(string); ok {
					message = msg
				}
				if d, ok := m["duration"].(float64); ok {
					duration = d
				}
				if px, ok := m["x"].(float64); ok {
					x = int(px)
				}
				if py, ok := m["y"].(float64); ok {
					y = int(py)
				}
				if fs, ok := m["fontsize"].(float64); ok {
					fontsize = int(fs)
				}
				if f, ok := m["fg"].(string); ok {
					fg = f
				}
				if b, ok := m["bg"].(string); ok {
					bg = b
				}
			}
			if message == "" {
				sendNack(id, "missing message")
				return
			}
			ctx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
			defer cancel2()
			if err := c.bipc.SendStyledMessage(ctx2, message, duration, x, y, fontsize, fg, bg); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdRequestSave:
		go func(id string) {
			log.Printf("handling request_save command")
			instanceID := ""
			if m, ok := cmd.Payload.(map[string]any); ok {
				if iid, ok := m["instance_id"].(string); ok {
					instanceID = iid
				}
			}
			log.Printf("request_save for instanceID=%s", instanceID)
			if instanceID == "" {
				sendNack(id, "missing instance_id")
				return
			}

			// Check if IPC is ready
			if !c.bipc.IsReady() {
				log.Printf("IPC not ready, cannot send SAVE command")
				sendNack(id, "IPC not ready")
				return
			}

			// Save the current state
			log.Printf("about to send SAVE command to BizHawk")
			if err := c.bipc.SendSave(ctx); err != nil {
				log.Printf("SendSave failed: %v", err)
				sendNack(id, "save failed: "+err.Error())
				return
			}
			log.Printf("save command sent to BizHawk")

			// Upload the save state
			log.Printf("about to upload save state for instanceID=%s", instanceID)
			if err := c.api.UploadSaveState(instanceID); err != nil {
				log.Printf("UploadSaveState failed: %v", err)
				sendNack(id, "upload failed: "+err.Error())
				return
			}
			log.Printf("save state uploaded for instanceID=%s", instanceID)
			sendAck(id)
		}(cmd.ID)
	case types.CmdStateUpdate:
		// Handle plugin settings updates
		go func() {
			if payload, ok := cmd.Payload.(map[string]any); ok {
				if pluginName, ok := payload["plugin_name"].(string); ok {
					if settingsMap, ok := payload["settings"].(map[string]any); ok {
						// Convert map[string]any to map[string]string
						settings := make(map[string]string)
						for k, v := range settingsMap {
							if str, ok := v.(string); ok {
								settings[k] = str
							} else {
								settings[k] = fmt.Sprintf("%v", v)
							}
						}
						// Save plugin settings
						if err := savePluginSettingsToFile(pluginName, settings); err != nil {
							log.Printf("failed to save plugin settings for %s: %v", pluginName, err)
						} else {
							log.Printf("updated plugin settings for %s", pluginName)
							// Notify BizHawk Lua script to reload plugin settings
							ctx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
							defer cancel2()
							if err := c.bipc.SendPluginSettings(ctx2, pluginName); err != nil {
								log.Printf("failed to send PLUGIN_SETTINGS command to BizHawk for %s: %v", pluginName, err)
							} else {
								log.Printf("sent PLUGIN_SETTINGS command to BizHawk for %s", pluginName)
							}
						}
					}
				}
			}
		}()
		sendAck(cmd.ID)
	case types.CmdPluginReload:
		// Handle plugin reload request
		go func() {
			if payload, ok := cmd.Payload.(map[string]any); ok {
				if pluginName, ok := payload["plugin_name"].(string); ok {
					log.Printf("Reloading plugin %s: syncing files and reloading in BizHawk", pluginName)

					// Create plugin sync manager
					httpClient := &http.Client{Timeout: 0}
					pluginSyncManager := NewPluginSyncManager(c.api, httpClient, c.cfg)

					// Sync the specific plugin (redownload files)
					// Since SyncPlugins syncs all plugins, we'll use it and then reload just this one
					if result, err := pluginSyncManager.SyncPlugins(); err != nil {
						log.Printf("failed to sync plugins for reload: %v", err)
					} else {
						log.Printf("plugin sync completed: %d total, %d downloaded, %d updated, %d removed in %v",
							result.TotalPlugins, result.Downloaded, result.Updated, result.Removed, result.Duration)

						// Notify BizHawk Lua script to fully reload this plugin (reload plugin.lua file)
						ctx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
						defer cancel2()
						if err := c.bipc.SendPluginReload(ctx2, pluginName); err != nil {
							log.Printf("failed to send PLUGIN_RELOAD command to BizHawk for %s: %v", pluginName, err)
						} else {
							log.Printf("sent PLUGIN_RELOAD command to BizHawk for %s", pluginName)
						}
					}
				}
			}
		}()
		sendAck(cmd.ID)
	case types.CmdFullscreenToggle:
		go func(id string) {
			log.Printf("handling fullscreen toggle command")
			// Execute keyTap("enter", "alt") to toggle fullscreen (Windows only)
			if err := keyTap("enter", "alt"); err != nil {
				log.Printf("failed to toggle fullscreen: %v", err)
				sendNack(id, "failed to toggle fullscreen: "+err.Error())
				return
			}
			log.Printf("fullscreen toggle executed (Alt+Enter)")
			sendAck(id)
		}(cmd.ID)
	case types.CmdCheckConfig:
		go func(id string) {
			log.Printf("handling check config command")
			// Read config.ini from BizHawk directory (same dir as EmuHawk.exe)
			bizhawkDir := filepath.Dir(c.cfg["bizhawk_path"])
			configPath := filepath.Join(bizhawkDir, "config.ini")
			configData, err := os.ReadFile(configPath)
			if err != nil {
				log.Printf("failed to read config.ini: %v", err)
				sendNack(id, "failed to read config: "+err.Error())
				return
			}

			// Parse JSON config
			var config map[string]any
			if err := json.Unmarshal(configData, &config); err != nil {
				log.Printf("failed to parse config JSON: %v", err)
				sendNack(id, "failed to parse config: "+err.Error())
				return
			}

			// Extract requested config keys
			var requestedKeys []string
			if pl, ok := cmd.Payload.(map[string]any); ok {
				if keys, ok := pl["config_keys"].([]any); ok {
					for _, key := range keys {
						if keyStr, ok := key.(string); ok {
							requestedKeys = append(requestedKeys, keyStr)
						}
					}
				}
			}

			// Extract values for requested keys
			configValues := make(map[string]any)
			for _, key := range requestedKeys {
				if val, exists := config[key]; exists {
					configValues[key] = val
				}
			}

			// Send config values back to server
			response := types.Command{
				Cmd:     types.CmdConfigResponse,
				Payload: map[string]any{"config_values": configValues},
				ID:      fmt.Sprintf("config-response-%d", time.Now().UnixNano()),
			}
			if err := c.writeJSON(response); err != nil {
				log.Printf("failed to send config response: %v", err)
				sendNack(id, "failed to send response: "+err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdUpdateConfig:
		go func(id string) {
			log.Printf("handling update config command")
			if pl, ok := cmd.Payload.(map[string]any); ok {
				if configUpdates, ok := pl["config_updates"].(string); ok {
					bizhawkDir := filepath.Dir(c.cfg["bizhawk_path"])
					configPath := filepath.Join(bizhawkDir, "config.ini")

					// Read current config
					configData, err := os.ReadFile(configPath)
					if err != nil {
						log.Printf("failed to read current config.ini: %v", err)
						sendNack(id, "failed to read current config: "+err.Error())
						return
					}

					// Parse current config
					var config map[string]any
					if err := json.Unmarshal(configData, &config); err != nil {
						log.Printf("failed to parse current config JSON: %v", err)
						sendNack(id, "failed to parse current config: "+err.Error())
						return
					}

					// Parse updates
					var updates map[string]any
					if err := json.Unmarshal([]byte(configUpdates), &updates); err != nil {
						log.Printf("failed to parse config updates: %v", err)
						sendNack(id, "failed to parse config updates: "+err.Error())
						return
					}

					// Check if BizHawk is running
					wasRunning := c.bipc.IsBizhawkLaunched()

					// Close BizHawk if it's running before updating config
					if wasRunning {
						if c.terminateBizhawkForConfig != nil {
							log.Printf("terminating BizHawk before config update")
							c.terminateBizhawkForConfig()
						} else {
							log.Printf("BizHawk is running but no terminate callback available")
						}
					}

					// Apply updates to config
					maps.Copy(config, updates)

					// Write updated config back
					updatedConfig, err := json.MarshalIndent(config, "", "  ")
					if err != nil {
						log.Printf("failed to marshal updated config: %v", err)
						sendNack(id, "failed to marshal config: "+err.Error())
						return
					}

					if err := os.WriteFile(configPath, updatedConfig, 0644); err != nil {
						log.Printf("failed to write updated config.ini: %v", err)
						sendNack(id, "failed to write config: "+err.Error())
						return
					}

					log.Printf("config updated successfully")

					// Send ACK
					sendAck(id)

					// Launch BizHawk if it was running before
					if wasRunning {
						if c.launchBizhawkForConfig != nil {
							// Small delay to allow BizHawk process cleanup before relaunch
							time.Sleep(500 * time.Millisecond)
							log.Printf("launching BizHawk after config update")
							c.launchBizhawkForConfig()
						} else if c.launchBizhawk != nil {
							// Fallback to normal launch if config-specific launch not available
							time.Sleep(500 * time.Millisecond)
							log.Printf("launching BizHawk after config update (using normal launch)")
							c.launchBizhawk()
							// Re-enable restart mode after launch to prevent the old BizHawk's
							// MonitorProcess from cancelling the client if it exits after launch
							if c.setRestartMode != nil {
								c.setRestartMode(true)
							}
						} else {
							log.Printf("BizHawk config updated but no launch callback available - manual launch required")
						}
					} else {
						log.Printf("BizHawk config updated - start BizHawk to apply changes")
					}
				} else {
					sendNack(id, "missing config_updates in payload")
				}
			} else {
				sendNack(id, "invalid payload format")
			}
		}(cmd.ID)
	default:
		sendAck(cmd.ID)
	}
}

func (c *Controller) EnsureSaveState(oldInstanceID, instanceID string) error {
	log.Println("Ensuring save state for instanceID:", instanceID)

	// Create saves directory if it doesn't exist
	if err := os.MkdirAll("./saves", 0755); err != nil {
		log.Printf("Failed to create saves directory: %v", err)
		return err
	}

	if oldInstanceID != "" {
		// 1. Upload old instance if it exists (current player's save state)
		go func() {
			log.Printf("Uploading save state for old instance: %s", oldInstanceID)
			err := c.api.UploadSaveState(oldInstanceID)
			if err != nil {
				log.Printf("Failed to upload old save state for instance %s: %v", oldInstanceID, err)
			} else {
				log.Printf("Successfully uploaded save state for instance %s", oldInstanceID)
			}
		}()
	}
	if instanceID == "" {
		log.Println("No instanceID provided, skipping save state orchestration")
		return nil
	}

	// 2. Download new instance save state (synchronous, blocking)
	log.Printf("Downloading save state for new instance: %s", instanceID)
	err := c.api.EnsureSaveState(instanceID)
	if err != nil {
		if err == ErrNotFound || err == ErrFileLocked {
			log.Printf("Save state for instance %s not available on server (this is OK, Lua will create one): %v", instanceID, err)
		} else {
			log.Printf("Failed to download save state for instance %s: %v", instanceID, err)
			return err
		}
	} else {
		log.Printf("Successfully downloaded save state for instance %s", instanceID)
	}

	return nil
}

// GetState returns the current game, instance ID and pending file
func (c *Controller) GetState() (game, instanceID, pending string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentGame, c.currentInstanceID, c.pendingFile
}

// GetMainGames returns a copy of the cached main games list
func (c *Controller) GetMainGames() []types.GameEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]types.GameEntry, len(c.mainGames))
	copy(result, c.mainGames)
	return result
}

// SetMainGames updates the cached main games list
func (c *Controller) SetMainGames(mainGames []types.GameEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.mainGames = make([]types.GameEntry, len(mainGames))
	copy(c.mainGames, mainGames)
}

// GetExtraFilesForGame returns the extra files for a given primary game file
func (c *Controller) GetExtraFilesForGame(game string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, entry := range c.mainGames {
		if entry.File == game {
			result := make([]string, len(entry.ExtraFiles))
			copy(result, entry.ExtraFiles)
			return result
		}
	}
	return nil
}

// clearDir removes all files from the specified directory
func clearDir(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("Failed to read directory %s: %v", dir, err)
		return
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		err := os.Remove(filepath.Join(dir, file.Name()))
		if err != nil {
			log.Printf("Failed to remove file %s from %s: %v", file.Name(), dir, err)
		}
	}
}

// ClearSaves removes all save files from the ./saves directory and BizHawk SaveRAM directories
func (c *Controller) ClearSaves() {
	// Clear local saves directory
	clearDir("./saves")

	// Clear BizHawk SaveRAM directories
	bizhawkDir := filepath.Dir(c.cfg["bizhawk_path"])
	subdirs := []string{"Gameboy/SaveRAM", "GBA/SaveRAM", "N64/SaveRAM", "NES/SaveRAM", "SNES/SaveRAM", "PSX/SaveRAM"}
	for _, subdir := range subdirs {
		clearDir(filepath.Join(bizhawkDir, subdir))
	}
}

// savePluginSettingsToFile saves plugin settings to settings.kv file
func savePluginSettingsToFile(pluginName string, settings map[string]string) error {
	pluginDir := filepath.Join("./plugins", pluginName)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugin dir: %w", err)
	}

	settingsKV := filepath.Join(pluginDir, "settings.kv")
	tmp := settingsKV + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Ensure status exists
	if _, exists := settings["status"]; !exists {
		settings["status"] = "disabled"
	}

	// Write status first
	if _, err := fmt.Fprintf(f, "status = %s\n", settings["status"]); err != nil {
		return fmt.Errorf("failed to write status: %w", err)
	}

	// Write other keys in sorted order
	keys := make([]string, 0, len(settings))
	for k := range settings {
		if k != "status" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		// Escape newlines in values
		val := strings.ReplaceAll(settings[k], "\n", "\\n")
		if _, err := fmt.Fprintf(f, "%s = %s\n", k, val); err != nil {
			return fmt.Errorf("failed to write setting %s: %w", k, err)
		}
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	if err := os.Rename(tmp, settingsKV); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// verifySaveWithRetry checks if the save file exists, has size > 0, and is a valid zip file.
// It retries up to 3 times with 200ms delays between attempts.
func (c *Controller) verifySaveWithRetry(instanceID string) error {
	filename := "./saves/" + instanceID + ".state"

	for attempt := range 3 {
		if attempt > 0 {
			time.Sleep(200 * time.Millisecond)
		}

		// file exists?
		if _, err := os.Stat(filename); err != nil {
			log.Printf("save file does not exist for instanceID=%s (attempt %d)", instanceID, attempt+1)
			continue
		}

		// file size > 0?
		info, err := os.Stat(filename)
		if err != nil {
			log.Printf("failed to stat save file for instanceID=%s (attempt %d): %v", instanceID, attempt+1, err)
			continue
		}
		if info.Size() == 0 {
			log.Printf("save file size is 0 for instanceID=%s (attempt %d)", instanceID, attempt+1)
			continue
		}

		// valid zip file?
		if file, err := os.Open(filename); err != nil {
			log.Printf("failed to open save file for instanceID=%s (attempt %d): %v", instanceID, attempt+1, err)
			continue
		} else {
			defer func() { _ = file.Close() }()
			if _, err := zip.NewReader(file, info.Size()); err != nil {
				log.Printf("save file is not a valid zip file for instanceID=%s (attempt %d): %v", instanceID, attempt+1, err)
				continue
			}
		}

		log.Printf("save file verification successful for instanceID=%s", instanceID)
		return nil
	}

	return fmt.Errorf("save file verification failed after 3 attempts for instanceID=%s", instanceID)
}
