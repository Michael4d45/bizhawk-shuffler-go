package clienthost

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/michael4d45/bizshuffle/obslog"
	"github.com/michael4d45/bizshuffle/protocol"
)

// Controller wires dependencies and handles incoming commands.
type Controller struct {
	cfg              Config
	bipc             *BizhawkIPC
	api              *API
	progressTracking *ProgressTrackingAPI
	writeJSON        func(protocol.Command) error
	// mainGames caches the server's main games list for extra_files lookup
	mainGames []protocol.GameEntry
	mu        sync.RWMutex // protects mainGames and state fields

	// state fields
	currentGame       string
	currentInstanceID string
	pendingFile       string

	// helloAck signals when hello has been acknowledged (first CmdGamesUpdate received)
	helloAck chan struct{}

	// pendingSwap holds a swap command received before BizHawk IPC is ready.
	pendingSwap protocol.Command

	// ipcMu serializes BizHawk IPC (swap, request_save) — TS commandChain parity.
	ipcMu sync.Mutex
}

func NewController(cfg Config, bipc *BizhawkIPC, api *API, writeJSON func(protocol.Command) error) *Controller {
	return NewControllerWithHelloAck(cfg, bipc, api, writeJSON, nil)
}

func NewControllerWithHelloAck(cfg Config, bipc *BizhawkIPC, api *API, writeJSON func(protocol.Command) error, helloAck chan struct{}) *Controller {
	c := &Controller{
		cfg:       cfg,
		bipc:      bipc,
		api:       api,
		writeJSON: writeJSON,
		mainGames: make([]protocol.GameEntry, 0),
		helloAck:  helloAck,
	}
	c.progressTracking = NewProgressTrackingAPI(api, c)
	return c
}

func payloadBool(payload any, key string) bool {
	m, ok := payload.(map[string]any)
	if !ok {
		return false
	}
	v, ok := m[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// OnBizhawkReady runs a swap that arrived before Lua IPC was ready.
func (c *Controller) OnBizhawkReady(ctx context.Context) {
	c.mu.Lock()
	pending := c.pendingSwap
	c.pendingSwap = protocol.Command{}
	c.mu.Unlock()
	if pending.Cmd == "" {
		return
	}
	c.Handle(ctx, pending)
}

// Handle processes a single incoming command. It launches goroutines for
// commands that should run asynchronously (keeps original behavior).
func (c *Controller) Handle(ctx context.Context, cmd protocol.Command) {
	sendAck := func(id string) { _ = c.writeJSON(protocol.Command{Cmd: protocol.CmdAck, ID: id}) }
	sendNack := func(id, reason string) {
		_ = c.writeJSON(protocol.Command{Cmd: protocol.CmdNack, ID: id, Payload: map[string]string{"reason": reason}})
	}

	switch cmd.Cmd {
	case protocol.CmdResume:
		go func(id string) {
			ctx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
			defer cancel2()
			if err := c.bipc.SendResume(ctx2); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case protocol.CmdPause:
		go func(id string) {
			if err := c.bipc.SendPause(ctx); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case protocol.CmdSwap:
		go func(swapCmd protocol.Command) {
			c.ipcMu.Lock()
			defer c.ipcMu.Unlock()
			if c.bipc != nil && !c.bipc.IsReady() {
				log.Printf("swap deferred until Lua HELLO (game=%v)", swapCmd.Payload)
				obslog.Event(obslog.Swap, "deferred", map[string]string{
					"reason": "lua_not_ready",
					"game":   fmt.Sprintf("%v", swapCmd.Payload),
				})
				c.mu.Lock()
				c.pendingSwap = swapCmd
				c.mu.Unlock()
				return
			}
			id := swapCmd.ID
			game := ""
			instanceID := ""
			skipSave := payloadBool(swapCmd.Payload, "skip_save")
			if m, ok := swapCmd.Payload.(map[string]any); ok {
				if g, ok := m["game"].(string); ok {
					game = g
				}
				if iid, ok := m["instance_id"].(string); ok {
					instanceID = iid
				}
			}
			if game == "" {
				log.Printf("swap command has empty game — acking without sending SWAP to Lua (check server assignment / admin games)")
				obslog.Event(obslog.Swap, "skip", map[string]string{
					"reason": "empty_game_from_server",
				})
				sendAck(id)
				return
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
			obslog.Event(obslog.Swap, "lua_send", map[string]string{
				"game": game, "instance_id": instanceID, "skip_save": fmt.Sprintf("%v", skipSave),
			})

			if c.bipc.IsReady() && !skipSave {
				_ = c.bipc.SendSave(ctx, oldInstanceID)
				if err := c.verifySaveWithRetry(oldInstanceID); err != nil {
					sendNack(id, "save verification failed: "+err.Error())
					return
				}
			}
			if !skipSave {
				if err := c.EnsureSaveState(oldInstanceID, instanceID); err != nil {
					sendNack(id, "save state orchestration failed: "+err.Error())
					return
				}
			} else if instanceID != "" {
				// Server already collected outgoing saves via request_save; still fetch the
				// incoming instance from the host so Lua does not load a stale local file.
				if err := c.downloadSaveState(instanceID); err != nil {
					sendNack(id, "save download failed: "+err.Error())
					return
				}
			}

			if err := c.bipc.SendSwap(ctx, game, instanceID); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd)
	case protocol.CmdClearSaves:
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
	case protocol.CmdGamesUpdate:
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
			var mainGames []protocol.GameEntry

			if m, ok := payload.(map[string]any); ok {
				// Parse and cache main_games first
				if mg, ok := m["main_games"].([]any); ok {
					for _, mei := range mg {
						if em, ok := mei.(map[string]any); ok {
							var entry protocol.GameEntry
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
			_ = c.writeJSON(protocol.Command{Cmd: protocol.CmdGamesUpdateAck, ID: fmt.Sprintf("%d", time.Now().UnixNano()), Payload: ackPayload})
		}(cmd.Payload)
	case protocol.CmdMessage:
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
	case protocol.CmdRequestSave:
		go func(id string) {
			c.ipcMu.Lock()
			defer c.ipcMu.Unlock()
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
			if err := c.bipc.SendSave(ctx, instanceID); err != nil {
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
	case protocol.CmdStateUpdate:
		go func(payload any) {
			pluginName, settings, ok := ParsePluginSettingsPayload(payload)
			if !ok {
				return
			}
			if err := ApplyPluginSettingsUpdate(c.bipc, pluginName, settings); err != nil {
				log.Printf("failed to apply plugin settings for %s: %v", pluginName, err)
			}
		}(cmd.Payload)
		sendAck(cmd.ID)
	case protocol.CmdPluginReload:
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
	default:
		sendAck(cmd.ID)
	}
}

func (c *Controller) EnsureSaveState(oldInstanceID, instanceID string) error {
	log.Println("Ensuring save state for instanceID:", instanceID)

	if oldInstanceID != "" {
		// Upload old instance if it exists (current player's save state).
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
	return c.downloadSaveState(instanceID)
}

// downloadSaveState fetches the latest save for instanceID from the server into ./saves.
func (c *Controller) downloadSaveState(instanceID string) error {
	if instanceID == "" {
		return nil
	}
	if err := os.MkdirAll("./saves", 0755); err != nil {
		log.Printf("Failed to create saves directory: %v", err)
		return err
	}
	log.Printf("Downloading save state for instance: %s", instanceID)
	err := c.api.EnsureSaveState(instanceID)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrFileLocked) {
			log.Printf("Save state for instance %s not available on server (this is OK, Lua will create one): %v", instanceID, err)
			return nil
		}
		log.Printf("Failed to download save state for instance %s: %v", instanceID, err)
		return err
	}
	log.Printf("Successfully downloaded save state for instance %s", instanceID)
	return nil
}

// GetState returns the current game, instance ID and pending file
func (c *Controller) GetState() (game, instanceID, pending string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentGame, c.currentInstanceID, c.pendingFile
}

// GetMainGames returns a copy of the cached main games list
func (c *Controller) GetMainGames() []protocol.GameEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]protocol.GameEntry, len(c.mainGames))
	copy(result, c.mainGames)
	return result
}

// SetMainGames updates the cached main games list
func (c *Controller) SetMainGames(mainGames []protocol.GameEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.mainGames = make([]protocol.GameEntry, len(mainGames))
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

// verifySaveWithRetry waits for BizHawk to write a valid savestate after SAVE.
func (c *Controller) verifySaveWithRetry(instanceID string) error {
	if instanceID == "" {
		return fmt.Errorf("missing instance id for save verification")
	}
	filename := "./saves/" + instanceID + ".state"

	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			time.Sleep(200 * time.Millisecond)
		}
		if err := verifySaveFilePath(filename); err != nil {
			lastErr = err
			log.Printf("save verify instanceID=%s attempt %d: %v", instanceID, attempt+1, err)
			continue
		}
		log.Printf("save file verification successful for instanceID=%s", instanceID)
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("save file verification failed for instanceID=%s: %w", instanceID, lastErr)
	}
	return fmt.Errorf("save file verification failed after 3 attempts for instanceID=%s", instanceID)
}
