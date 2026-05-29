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
