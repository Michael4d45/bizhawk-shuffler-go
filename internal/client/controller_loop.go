package client

import (
	"context"
	"log"
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
func RunControllerLoop(ctx context.Context, cfg Config, wsClient WSClientLike, bipc BizhawkIPCLike, dl *internal.Downloader, writeJSON func(types.Command) error, uploadSave func(string, string, string) error, downloadSave func(context.Context, string, string) error, ipcReadyMu *sync.Mutex, ipcReady *bool) <-chan struct{} {
	// incoming commands channel (buffered to avoid blocking the WS reader)
	cmdCh := make(chan types.Command, 64)
	wsClient.RegisterHandler(func(cmd types.Command) {
		select {
		case cmdCh <- cmd:
		default:
			log.Printf("incoming command dropped: %v", cmd.Cmd)
		}
	})

	// send initial hello
	hello := types.Command{Cmd: types.CmdHello, Payload: map[string]string{"name": cfg["name"]}, ID: ""}
	_ = writeJSON(hello)

	controller := NewController(cfg, bipc, dl, writeJSON, uploadSave, downloadSave, ipcReadyMu, ipcReady)

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			select {
			case <-ctx.Done():
				return
			case cmd := <-cmdCh:
				log.Printf("server->client cmd: %s", cmd.Cmd)
				controller.Handle(ctx, cmd)
			}
		}
	}()
	return readDone
}
