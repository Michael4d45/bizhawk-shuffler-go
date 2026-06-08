package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/clienthost"
	"github.com/michael4d45/bizshuffle/protocol"
	"github.com/michael4d45/bizshuffle/testing/fakes"
)

func seedTestPlugin(t *testing.T, dataDir, name string) {
	t.Helper()
	pluginDir := filepath.Join(dataDir, "plugins", name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "meta.kv"), []byte("name = "+name+"\nversion = 1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "settings.kv"), []byte("status = enabled\ncommand_type = swap\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPluginSettingsBroadcastToPlayer(t *testing.T) {
	ts := StartTestServer(t)
	seedTestPlugin(t, ts.DataDir, "memory-tracker")

	ws := NewWSTestClient(ts.URL)
	if err := ws.Connect(); err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := ws.Hello("p1", true); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"status": "enabled", "command_type": "swap_me"})
	res, err := http.Post(ts.URL+"/api/plugins/memory-tracker/settings", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST settings status %d", res.StatusCode)
	}

	cmd, err := ws.WaitFor(func(c protocol.Command) bool {
		if c.Cmd != protocol.CmdStateUpdate {
			return false
		}
		m, ok := c.Payload.(map[string]any)
		if !ok {
			return false
		}
		return m["plugin_name"] == "memory-tracker"
	}, 3*time.Second)
	if err != nil {
		t.Fatalf("player did not receive state_update: %v", err)
	}
	m := cmd.Payload.(map[string]any)
	settings := m["settings"].(map[string]any)
	if settings["command_type"] != "swap_me" {
		t.Fatalf("command_type = %v", settings["command_type"])
	}
}

func TestPluginSettingsLiveUpdateReachesBizHawk(t *testing.T) {
	ts := StartTestServer(t)
	const pluginName = "memory-tracker"
	const player = "player1"
	seedTestPlugin(t, ts.DataDir, pluginName)
	base := ts.URL
	postAddPlayer(t, base, player)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(ts.DataDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	if err := clienthost.EnsureDataDirs(ts.DataDir); err != nil {
		t.Fatal(err)
	}

	bipc, err := clienthost.NewBizhawkIPC(ts.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bipc.Close() })

	peer, err := fakes.StartFakeLuaPeerOnPort(bipc.Port(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	bipc.SetBizhawkLaunched(true)
	if err := bipc.Start(ctx); err != nil {
		t.Fatal(err)
	}

	cfg := clienthost.Config{"name": player, "server": base}
	api := clienthost.NewAPI(base, &http.Client{Timeout: 30 * time.Second}, cfg)
	ws := clienthost.NewWSClient(HTTPToWS(base), api, bipc)

	done := make(chan struct{})
	go func() {
		ws.Start(ctx, cfg)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timeout waiting for ws client hello")
	}

	body, _ := json.Marshal(map[string]string{"status": "enabled", "command_type": "swap_me"})
	res, err := http.Post(base+"/api/plugins/"+pluginName+"/settings", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST settings status %d", res.StatusCode)
	}

	if !peer.WaitForCommand("PLUGIN_SETTINGS", 8*time.Second) {
		t.Fatalf("BizHawk never received PLUGIN_SETTINGS; cmds=%v", peer.ReceivedCommands())
	}

	kvPath := filepath.Join(ts.DataDir, "plugins", pluginName, "settings.kv")
	kvBody, err := os.ReadFile(kvPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(kvBody), "command_type = swap_me") {
		t.Fatalf("client settings.kv not updated: %s", kvBody)
	}

	ws.Stop()
}

func TestPluginSettingsMergePreservesExistingKeys(t *testing.T) {
	ts := StartTestServer(t)
	seedTestPlugin(t, ts.DataDir, "memory-tracker")

	body, _ := json.Marshal(map[string]string{"status": "disabled"})
	res, err := http.Post(ts.URL+"/api/plugins/memory-tracker/settings", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST settings status %d", res.StatusCode)
	}

	kvPath := filepath.Join(ts.DataDir, "plugins", "memory-tracker", "settings.kv")
	kvBody, err := os.ReadFile(kvPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(kvBody)
	if !strings.Contains(text, "status = disabled") {
		t.Fatalf("status not disabled: %s", text)
	}
	if !strings.Contains(text, "command_type = swap") {
		t.Fatalf("command_type was wiped: %s", text)
	}
}
