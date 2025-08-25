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
// updates ipcReady state on HELLO, and calls bipc.SendSync. It will call
// cancel() if a disconnection is detected or when the incoming channel
// closes.
func StartIPCGoroutine(ctx context.Context, bipc *internal.BizhawkIPC, cfgName string, fetchServerState func() (bool, string), ipcReadyMu *sync.Mutex, ipcReady *bool, cancel context.CancelFunc) {
	go func() {
		for line := range bipc.Incoming() {
			if line == internal.MsgDisconnected || line == "__BIZHAWK_IPC_DISCONNECTED__" {
				log.Printf("bizhawk ipc: disconnected detected from readLoop")
				cancel()
				break
			}
			log.Printf("lua: %s", line)
			if strings.HasPrefix(line, "HELLO") {
				log.Printf("received HELLO from lua, sending sync")
				ipcReadyMu.Lock()
				*ipcReady = true
				ipcReadyMu.Unlock()
				running, playerGame := fetchServerState()
				if playerGame == "" {
					log.Printf("no current game for player from server state; sending empty game")
				}
				ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
				if err := bipc.SendSync(ctx2, playerGame, running, time.Now().Unix()); err != nil {
					log.Printf("SendSync failed: %v", err)
				}
				cancel2()
			}
		}
		log.Printf("bizhawk ipc incoming channel closed or handler exited")
		cancel()
	}()
}
