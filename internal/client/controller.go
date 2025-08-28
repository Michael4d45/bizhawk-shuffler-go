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
	cfg       Config
	bipc      *BizhawkIPC
	api       *API
	writeJSON func(types.Command) error
}

func NewController(cfg Config, bipc *BizhawkIPC, api *API, writeJSON func(types.Command) error) *Controller {
	return &Controller{cfg: cfg, bipc: bipc, api: api, writeJSON: writeJSON}
}

// Handle processes a single incoming command. It launches goroutines for
// commands that should run asynchronously (keeps original behavior).
func (c *Controller) Handle(ctx context.Context, cmd types.Command) {
	sendAck := func(id string) { _ = c.writeJSON(types.Command{Cmd: types.CmdAck, ID: id}) }
	sendNack := func(id, reason string) {
		_ = c.writeJSON(types.Command{Cmd: types.CmdNack, ID: id, Payload: map[string]string{"reason": reason}})
	}

	switch cmd.Cmd {
	case types.CmdStart:
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
			// If no game provided, treat as a resume/unpause signal.
			if game == "" {
				ctx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
				defer cancel2()
				if err := c.bipc.SendResume(ctx2, nil); err != nil {
					sendNack(id, err.Error())
					return
				}
				sendAck(id)
				return
			} else {
				ctx2, cancel2 := context.WithTimeout(ctx, 30*time.Second)
				if err := c.api.EnsureFile(ctx2, game); err != nil {
					cancel2()
					sendNack(id, "download failed: "+err.Error())
					return
				}
				cancel2()
			}

			if err := c.EnsureSaveState(instanceID); err != nil {
				sendNack(id, "save state orchestration failed: "+err.Error())
				return
			}

			log.Printf("handling start command for game=%s", game)
			if err := c.bipc.SendStart(ctx, time.Now().Unix(), game, instanceID); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdPause:
		go func(id string) {
			if err := c.bipc.SendPause(ctx, nil); err != nil {
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
			if game == "" {
				sendNack(id, "missing game")
				return
			}
			ctx2, cancel2 := context.WithTimeout(ctx, 30*time.Second)
			log.Printf("ensuring ROM present for game=%s", game)
			if err := c.api.EnsureFile(ctx2, game); err != nil {
				cancel2()
				sendNack(id, "download failed: "+err.Error())
				return
			}
			cancel2()
			log.Printf("sending swap to lua for game=%s", game)

			if err := c.EnsureSaveState(instanceID); err != nil {
				sendNack(id, "save state orchestration failed: "+err.Error())
				return
			}

			if err := c.bipc.SendSwap(ctx, time.Now().Unix(), game, instanceID); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdClearSaves:
		go func(id string) {

			// delete files in ./saves directory
			files, err := os.ReadDir("./saves")
			if err != nil {
				log.Printf("Failed to read saves directory: %v", err)
			} else {
				for _, file := range files {
					err := os.RemoveAll(filepath.Join("./saves", file.Name()))
					if err != nil {
						log.Printf("Failed to delete save file %s: %v", file.Name(), err)
					}
				}
			}

			if err := c.bipc.SendMessage(ctx, "clear_saves"); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdReset:
		go func(id string) {
			if err := c.bipc.SendMessage(ctx, "reset"); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdGamesUpdate:
		go func(payload any) {
			required := make(map[string]struct{})
			// Build set of instance games we need
			instanceGames := make(map[string]struct{})
			if m, ok := payload.(map[string]any); ok {
				if gis, ok := m["game_instances"].([]any); ok {
					for _, gi := range gis {
						if im, ok := gi.(map[string]any); ok {
							if g, ok2 := im["game"].(string); ok2 && g != "" {
								instanceGames[g] = struct{}{}
								required[g] = struct{}{}
							}
						}
					}
				}
				// extras from main_games when primary is in instanceGames
				if mg, ok := m["main_games"].([]any); ok {
					for _, mei := range mg {
						if em, ok := mei.(map[string]any); ok {
							if f, ok := em["file"].(string); ok {
								if _, isActive := instanceGames[f]; isActive {
									if extras, ok := em["extra_files"].([]any); ok {
										for _, ex := range extras {
											if exs, ok := ex.(string); ok {
												required[exs] = struct{}{}
											}
										}
									}
								}
							}
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
					if err := c.api.EnsureFile(ctx2, fname); err != nil {
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
	default:
		sendAck(cmd.ID)
	}
}

func (c *Controller) EnsureSaveState(instanceID string) error {
	if instanceID == "" {
		log.Println("No instanceID provided, skipping save state orchestration")
		return nil
	}

	log.Println("Ensuring save state for instanceID:", instanceID)

	// Create saves directory if it doesn't exist
	if err := os.MkdirAll("./saves", 0755); err != nil {
		log.Printf("Failed to create saves directory: %v", err)
		return err
	}

	// 1. Upload old instance if it exists (current player's save state)
	if c.bipc.instanceID != "" && c.bipc.instanceID != instanceID {
		log.Printf("Uploading save state for old instance: %s", c.bipc.instanceID)
		err := c.api.UploadSaveState(c.bipc.instanceID)
		if err != nil {
			log.Printf("Failed to upload old save state for instance %s: %v", c.bipc.instanceID, err)
			// Don't return error here as this is not critical for the swap
		} else {
			log.Printf("Successfully uploaded save state for instance %s", c.bipc.instanceID)
		}
	}

	// 2. Download new instance save state (synchronous, blocking)
	log.Printf("Downloading save state for new instance: %s", instanceID)
	err := c.api.EnsureSaveState(instanceID)
	if err != nil {
		if err == ErrNotFound {
			log.Printf("Save state for instance %s not found on server (this is OK, Lua will create one)", instanceID)
		} else {
			log.Printf("Failed to download save state for instance %s: %v", instanceID, err)
			return err
		}
	} else {
		log.Printf("Successfully downloaded save state for instance %s", instanceID)
	}

	return nil
}
