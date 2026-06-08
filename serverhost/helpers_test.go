package serverhost

import (
	"os"
	"testing"

	"github.com/michael4d45/bizshuffle/protocol"
)

// chdirToTemp runs the test with cwd in an isolated directory (state.json, saves, roms).
func chdirToTemp(t *testing.T) {
	t.Helper()
	dataDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dataDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
}

func registerPlayerWSClient(s *Server, name string) *wsClient {
	client := &wsClient{sendCh: make(chan protocol.Command, 8)}
	s.withConnLock(func() {
		s.playerClients[name] = client
	})
	return client
}

// startFakePlayerClient registers a player WS client that completes request_save by
// marking the target instance ready (for save-mode swap orchestration tests).
func startFakePlayerClient(s *Server, name string) *wsClient {
	client := registerPlayerWSClient(s, name)
	go func() {
		for cmd := range client.sendCh {
			switch cmd.Cmd {
			case protocol.CmdRequestSave:
				instanceID := ""
				switch p := cmd.Payload.(type) {
				case map[string]string:
					instanceID = p["instance_id"]
				case map[string]any:
					if v, ok := p["instance_id"].(string); ok {
						instanceID = v
					}
				}
				if instanceID != "" {
					s.setInstanceFileState(instanceID, protocol.FileStateReady)
				}
			case protocol.CmdSwap:
				if cmd.ID != "" {
					s.withRLock(func() {
						if ch, ok := s.pending[cmd.ID]; ok {
							select {
							case ch <- "ack":
							default:
							}
						}
					})
				}
			}
		}
	}()
	return client
}
