package clienthost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestShellSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	SaveShellSettingsForm(dir, "0.0.0.0", "http://192.168.1.10:9090", "Alice", 9090)
	loaded := LoadShellSettings(dir)
	if loaded.BindHost != "0.0.0.0" || loaded.HostPort != 9090 {
		t.Fatalf("got %+v", loaded)
	}
	if loaded.ServerURL != "http://192.168.1.10:9090" || loaded.PlayerName != "Alice" {
		t.Fatalf("got %+v", loaded)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["bind_host"] != "0.0.0.0" || m["host_port"] != "9090" || m["name"] != "Alice" {
		t.Fatalf("raw %v", m)
	}
}

func TestShellSettingsPartialMerge(t *testing.T) {
	dir := t.TempDir()
	SaveShellSettingsForm(dir, "127.0.0.1", "http://127.0.0.1:8080", "", 8080)
	SaveShellSettings(dir, ShellSettings{PlayerName: "Bob"})
	loaded := LoadShellSettings(dir)
	if loaded.PlayerName != "Bob" || loaded.ServerURL != "http://127.0.0.1:8080" {
		t.Fatalf("got %+v", loaded)
	}
}

func TestShellSettingsDefaultsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	loaded := LoadShellSettings(dir)
	def := DefaultShellSettings()
	if loaded != def {
		t.Fatalf("got %+v want %+v", loaded, def)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); !os.IsNotExist(err) {
		t.Fatal("expected no config.json until save")
	}
}
