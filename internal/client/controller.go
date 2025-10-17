package client

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	mu        sync.RWMutex // protects mainGames
}

func NewController(cfg Config, bipc *BizhawkIPC, api *API, writeJSON func(types.Command) error) *Controller {
	c := &Controller{
		cfg:       cfg,
		bipc:      bipc,
		api:       api,
		writeJSON: writeJSON,
		mainGames: make([]types.GameEntry, 0),
	}
	c.progressTracking = NewProgressTrackingAPI(api, c)
	return c
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
			if game != "" {
				ctx2, cancel2 := context.WithTimeout(ctx, 30*time.Second)
				log.Printf("ensuring ROM present for game=%s", game)
				if err := c.progressTracking.EnsureFileWithProgress(ctx2, game); err != nil {
					cancel2()
					sendNack(id, "download failed: "+err.Error())
					return
				}
				cancel2()
			}
			log.Printf("sending swap to lua for game=%s", game)

			_ = c.bipc.SendSave(ctx)
			if err := c.EnsureSaveState(instanceID); err != nil {
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
	default:
		sendAck(cmd.ID)
	}
}

func (c *Controller) EnsureSaveState(instanceID string) error {
	log.Println("Ensuring save state for instanceID:", instanceID)

	// Create saves directory if it doesn't exist
	if err := os.MkdirAll("./saves", 0755); err != nil {
		log.Printf("Failed to create saves directory: %v", err)
		return err
	}

	// 1. Upload old instance if it exists (current player's save state)
	go func() {
		if c.bipc.instanceID != "" {
			log.Printf("Uploading save state for old instance: %s", c.bipc.instanceID)
			err := c.api.UploadSaveState(c.bipc.instanceID)
			if err != nil {
				log.Printf("Failed to upload old save state for instance %s: %v", c.bipc.instanceID, err)
			} else {
				log.Printf("Successfully uploaded save state for instance %s", c.bipc.instanceID)
			}
		}
	}()

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
