package client

import (
	"context"
	"log"
	"runtime"
	"sync"

	"github.com/michael4d45/bizshuffle/internal"
	"github.com/michael4d45/bizshuffle/internal/types"
)

// WSClientLike is a minimal interface around the WS client used by the
// controller wiring. Defining an interface here allows tests to inject a
// fake implementation.
type WSClientLike interface {
	Start(ctx context.Context)
	Stop()
	Send(types.Command) error
	RegisterHandler(func(types.Command))
}

// RunControllerLoop wires the websocket client and bizhawk ipc into the
// Controller and starts the read loop. It returns a channel that is closed
// when the read loop exits.
func RunControllerLoop(ctx context.Context, cfg Config, wsClient WSClientLike, bipc BizhawkIPCLike, dl *internal.Downloader, writeJSON func(types.Command) error, ipcReadyMu *sync.Mutex, ipcReady *bool) <-chan struct{} {
	// incoming commands channel (buffered to avoid blocking the WS reader)
	cmdCh := make(chan types.Command, 64)
	wsClient.RegisterHandler(func(cmd types.Command) {
		select {
		case cmdCh <- cmd:
		default:
			var ir bool
			ipcReadyMu.Lock()
			ir = *ipcReady
			ipcReadyMu.Unlock()
			log.Printf("incoming command dropped: %v; goroutines=%d ipcReady=%v", cmd.Cmd, runtime.NumGoroutine(), ir)
		}
	})

	// send initial hello
	hello := types.Command{Cmd: types.CmdHello, Payload: map[string]string{"name": cfg["name"]}, ID: ""}
	_ = writeJSON(hello)

	controller := NewController(cfg, bipc, dl, writeJSON, ipcReadyMu, ipcReady)

	readDone := make(chan struct{})
	go func() {
		defer func() {
			log.Printf("controller read loop exiting; closing readDone")
			close(readDone)
		}()
		for {
			select {
			case <-ctx.Done():
				log.Printf("controller read loop: ctx.Done received; exiting")
				return
			case cmd, ok := <-cmdCh:
				if !ok {
					log.Printf("controller read loop: cmdCh closed; exiting")
					return
				}
				log.Printf("server->client cmd: %s", cmd.Cmd)
				// protect handler from panics
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("controller.Handle panic: %v", r)
						}
					}()
					controller.Handle(ctx, cmd)
				}()
			}
		}
	}()
	return readDone
}
