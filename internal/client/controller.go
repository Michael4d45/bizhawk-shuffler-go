package client

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/michael4d45/bizshuffle/internal"
	"github.com/michael4d45/bizshuffle/internal/types"
)

// BizhawkIPCLike is a minimal interface around internal.BizhawkIPC used by the controller
type BizhawkIPCLike interface {
	Start(ctx context.Context) error
	Close() error
	Incoming() <-chan string
	SendSync(ctx context.Context, game string, running bool, ts int64) error
	SendStart(ctx context.Context, ts int64, game string) error
	SendPause(ctx context.Context, at *int64) error
	SendResume(ctx context.Context, at *int64) error
	SendSave(ctx context.Context, localPath string) error
	SendSwap(ctx context.Context, ts int64, game string) error
	SendCommand(ctx context.Context, parts ...string) error
	SendMessage(ctx context.Context, msg string) error
}

// Controller wires dependencies and handles incoming commands.
type Controller struct {
	cfg        Config
	bipc       BizhawkIPCLike
	dl         *internal.Downloader
	writeJSON  func(types.Command) error
	upload     func(localPath, player, game string) error
	download   func(ctx context.Context, player, filename string) error
	ipcReadyMu *sync.Mutex
	ipcReady   *bool
}

func NewController(cfg Config, bipc BizhawkIPCLike, dl *internal.Downloader, writeJSON func(types.Command) error, upload func(string, string, string) error, download func(context.Context, string, string) error, ipcReadyMu *sync.Mutex, ipcReady *bool) *Controller {
	return &Controller{cfg: cfg, bipc: bipc, dl: dl, writeJSON: writeJSON, upload: upload, download: download, ipcReadyMu: ipcReadyMu, ipcReady: ipcReady}
}

// Handle processes a single incoming command. It launches goroutines for
// commands that should run asynchronously (keeps original behavior).
func (c *Controller) Handle(ctx context.Context, cmd types.Command) {
	sendAck := func(id string) { _ = c.writeJSON(types.Command{Cmd: types.CmdAck, ID: id}) }
	sendNack := func(id, reason string) {
		_ = c.writeJSON(types.Command{Cmd: types.CmdNack, ID: id, Payload: map[string]string{"reason": reason}})
	}
	sendNackDetailed := func(id, shortReason, filePath string, errDetails error) {
		diag := map[string]any{"reason": shortReason}
		if filePath != "" {
			if fi, err := os.Stat(filePath); err == nil {
				diag["file_exists"] = true
				diag["file_size"] = fi.Size()
				diag["file_modtime"] = fi.ModTime().Unix()
			} else {
				diag["file_exists"] = false
				diag["file_stat_error"] = err.Error()
			}
		}
		c.ipcReadyMu.Lock()
		diag["ipc_ready"] = *c.ipcReady
		c.ipcReadyMu.Unlock()
		if errDetails != nil {
			diag["error_detail"] = errDetails.Error()
		}
		_ = c.writeJSON(types.Command{Cmd: types.CmdNack, ID: id, Payload: diag})
	}

	switch cmd.Cmd {
	case types.CmdStart:
		go func(id string) {
			game := ""
			if m, ok := cmd.Payload.(map[string]any); ok {
				if g, ok := m["game"].(string); ok {
					game = g
				}
			}
			if game != "" {
				ctx2, cancel2 := context.WithTimeout(ctx, 30*time.Second)
				if err := c.dl.EnsureFile(ctx2, game); err != nil {
					cancel2()
					sendNack(id, "download failed: "+err.Error())
					return
				}
				cancel2()
			}
			log.Printf("handling start command for game=%s", game)
			if err := c.bipc.SendStart(ctx, time.Now().Unix(), game); err != nil {
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
	case types.CmdResume:
		go func(id string) {
			if err := c.bipc.SendResume(ctx, nil); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdSwap:
		go func(id string) {
			game := ""
			if m, ok := cmd.Payload.(map[string]any); ok {
				if g, ok := m["game"].(string); ok {
					game = g
				}
			}
			if game == "" {
				sendNack(id, "missing game")
				return
			}
			player := c.cfg["name"]
			saveDir := filepath.Join("./saves", player)
			if err := os.MkdirAll(saveDir, 0755); err != nil {
				sendNack(id, "mkdir failed: "+err.Error())
				return
			}
			localSave := filepath.Join(saveDir, game+".state")
			log.Printf("requesting save to localSave=%s", localSave)
			if err := c.bipc.SendSave(ctx, localSave); err != nil {
				log.Printf("SendSave failed: %v", err)
				sendNackDetailed(id, "save failed", localSave, err)
			}
			if err := c.upload(localSave, player, game); err != nil {
				log.Printf("uploadSave failed: %v", err)
				if os.IsNotExist(err) {
					log.Printf("local save missing, continuing without upload: %s", localSave)
				} else {
					sendNackDetailed(id, "upload failed", localSave, err)
					return
				}
			}
			ctx2, cancel2 := context.WithTimeout(ctx, 30*time.Second)
			log.Printf("ensuring ROM present for game=%s", game)
			if err := c.dl.EnsureFile(ctx2, game); err != nil {
				cancel2()
				sendNack(id, "download failed: "+err.Error())
				return
			}
			cancel2()
			log.Printf("sending swap to lua for game=%s", game)
			if err := c.bipc.SendSwap(ctx, time.Now().Unix(), game); err != nil {
				sendNack(id, err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdDownloadSave:
		go func(id string) {
			player := ""
			file := ""
			if m, ok := cmd.Payload.(map[string]any); ok {
				if p, ok := m["player"].(string); ok {
					player = p
				}
				if f, ok := m["file"].(string); ok {
					file = f
				}
			}
			if player == "" || file == "" {
				sendNack(id, "missing player or file")
				return
			}
			ctx2, cancel2 := context.WithTimeout(ctx, 30*time.Second)
			defer cancel2()
			log.Printf("handling download_save for player=%s file=%s", player, file)
			if err := c.download(ctx2, player, file); err != nil {
				if errors.Is(err, ErrNotFound) {
					log.Printf("download_save: remote save not found for player=%s file=%s; acking without LOAD", player, file)
					sendAck(id)
					return
				}
				sendNack(id, "download save failed: "+err.Error())
				return
			}
			localPath := filepath.Join("./saves", player, file)
			if err := c.bipc.SendCommand(ctx2, "LOAD", localPath); err != nil {
				sendNack(id, "load failed: "+err.Error())
				return
			}
			sendAck(id)
		}(cmd.ID)
	case types.CmdClearSaves:
		go func(id string) {
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
			active := make(map[string]struct{})
			if m, ok := payload.(map[string]any); ok {
				if gs, ok := m["games"].([]any); ok {
					for _, gi := range gs {
						if sname, ok := gi.(string); ok {
							required[sname] = struct{}{}
							active[sname] = struct{}{}
						}
					}
				}
				if mg, ok := m["main_games"].([]any); ok {
					for _, mei := range mg {
						if em, ok := mei.(map[string]any); ok {
							if f, ok := em["file"].(string); ok {
								if _, isActive := active[f]; isActive {
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
					if err := c.dl.EnsureFile(ctx2, fname); err != nil {
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
