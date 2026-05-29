package clienthost

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureServerLuaFromEmbed(t *testing.T) {
	if len(embeddedServerLua) == 0 {
		t.Fatal("embedded server.lua is empty")
	}
	dir := t.TempDir()
	path, err := EnsureServerLua(dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "server.lua") {
		t.Fatalf("path %q", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, embeddedServerLua) {
		t.Fatal("dest does not match embedded asset")
	}
}

// TestEnsureServerLuaAfterChdir mirrors StartJoinSession: chdir into dataDir with no assets/ nearby.
func TestEnsureServerLuaAfterChdir(t *testing.T) {
	if len(embeddedServerLua) == 0 {
		t.Fatal("embedded server.lua is empty")
	}
	dataDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dataDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	// Relative assets/server.lua must not exist in an empty temp cwd.
	if _, err := os.Stat(filepath.Join("assets", "server.lua")); err == nil {
		t.Fatal("unexpected assets/server.lua in temp data dir")
	}

	path, err := EnsureServerLua(dataDir)
	if err != nil {
		t.Fatalf("EnsureServerLua after chdir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("written server.lua missing: %v", err)
	}
}
