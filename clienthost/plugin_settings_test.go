package clienthost

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michael4d45/bizshuffle/protocol"
	"github.com/michael4d45/bizshuffle/testing/fakes"
)

func TestParsePluginSettingsPayload(t *testing.T) {
	name, settings, ok := ParsePluginSettingsPayload(map[string]any{
		"plugin_name": "memory-tracker",
		"settings": map[string]any{
			"status":        "enabled",
			"command_type":  "swap_me",
			"enabled_types": "door",
		},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if name != "memory-tracker" {
		t.Fatalf("name %q", name)
	}
	if settings["command_type"] != "swap_me" {
		t.Fatalf("command_type %q", settings["command_type"])
	}
	if _, _, ok := ParsePluginSettingsPayload(map[string]any{"updated_at": "now"}); ok {
		t.Fatal("admin updated_at payload should not parse as plugin settings")
	}
}

func TestApplyPluginSettingsUpdateWritesFileAndNotifiesBizHawk(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	bipc, err := NewBizhawkIPC(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bipc.Close() })

	peer, err := fakes.StartFakeLuaPeerOnPort(bipc.Port(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	bipc.SetBizhawkLaunched(true)
	if err := bipc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	settings := map[string]string{
		"status":       "enabled",
		"command_type": "swap_me",
	}
	if err := ApplyPluginSettingsUpdate(bipc, "memory-tracker", settings); err != nil {
		t.Fatal(err)
	}

	kvPath := filepath.Join(dir, "plugins", "memory-tracker", "settings.kv")
	body, err := os.ReadFile(kvPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "command_type = swap_me") {
		t.Fatalf("settings.kv missing swap_me: %s", body)
	}
	if !peer.WaitForCommand("PLUGIN_SETTINGS", 2*time.Second) {
		t.Fatalf("BizHawk did not receive PLUGIN_SETTINGS; cmds=%v", peer.ReceivedCommands())
	}
}

func TestControllerHandlesPluginSettingsStateUpdate(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	bipc, err := NewBizhawkIPC(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bipc.Close() })

	peer, err := fakes.StartFakeLuaPeerOnPort(bipc.Port(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	bipc.SetBizhawkLaunched(true)
	if err := bipc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	ctrl := NewController(Config{}, bipc, nil, func(protocol.Command) error { return nil })
	done := make(chan struct{})
	go func() {
		ctrl.Handle(ctx, protocol.Command{
			Cmd: protocol.CmdStateUpdate,
			ID:  "settings-1",
			Payload: map[string]any{
				"plugin_name": "memory-tracker",
				"settings": map[string]any{
					"status":       "enabled",
					"command_type": "swap_me",
				},
			},
		})
		close(done)
	}()
	<-done
	time.Sleep(200 * time.Millisecond)

	if !peer.WaitForCommand("PLUGIN_SETTINGS", 3*time.Second) {
		t.Fatalf("controller did not send PLUGIN_SETTINGS; cmds=%v", peer.ReceivedCommands())
	}
}
