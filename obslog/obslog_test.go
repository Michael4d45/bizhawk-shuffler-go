package obslog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEventWritesNDJSON(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Close)
	Event(Join, "test_event", map[string]string{"player": "alice", "ws_url": "ws://127.0.0.1:8080/ws"})

	path := filepath.Join(dir, "debug-trace.ndjson")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"event":"test_event"`) {
		t.Fatalf("missing event in trace: %s", raw)
	}
	if !strings.Contains(string(raw), `"player":"alice"`) {
		t.Fatalf("missing player in trace: %s", raw)
	}
}
