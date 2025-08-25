package client

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/michael4d45/bizshuffle/internal"
)

// StartIPCGoroutine starts a goroutine that reads lines from bipc.Incoming(),
// updates ipcReady state on HELLO, and calls bipc.SendSync. It does NOT call
// the provided cancel function; instead it marks `ipcReady=false` on
// disconnects or when the incoming channel closes and lets reconnect logic
// and higher-level shutdown handling decide whether to exit.
func StartIPCGoroutine(ctx context.Context, bipc *internal.BizhawkIPC, cfgName string, fetchServerState func() (bool, string), ipcReadyMu *sync.Mutex, ipcReady *bool) {
	go func() {
		for line := range bipc.Incoming() {
			if line == internal.MsgDisconnected || line == "__BIZHAWK_IPC_DISCONNECTED__" {
				log.Printf("bizhawk ipc: disconnected detected from readLoop (ipc handler); marking ipcReady=false and continuing")
				ipcReadyMu.Lock()
				*ipcReady = false
				ipcReadyMu.Unlock()
				// don't cancel the main context here; allow reconnect logic to run
				continue
			}
			log.Printf("lua incoming: %s", line)
			if strings.HasPrefix(line, "HELLO") {
				log.Printf("ipc handler: received HELLO from lua, sending SYNC")
				ipcReadyMu.Lock()
				*ipcReady = true
				ipcReadyMu.Unlock()
				running, playerGame := fetchServerState()
				if playerGame == "" {
					log.Printf("ipc handler: no current game for player from server state; sending empty game")
				}
				ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
				if err := bipc.SendSync(ctx2, playerGame, running, time.Now().Unix()); err != nil {
					log.Printf("ipc handler: SendSync failed: %v", err)
				} else {
					log.Printf("ipc handler: SendSync succeeded (game=%q running=%v)", playerGame, running)
				}
				cancel2()
			}
		}
		log.Printf("bizhawk ipc: incoming channel closed or handler goroutine exiting; marking ipcReady=false")
		ipcReadyMu.Lock()
		*ipcReady = false
		ipcReadyMu.Unlock()
	}()
}
