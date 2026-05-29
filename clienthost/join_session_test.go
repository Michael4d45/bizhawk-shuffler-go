package clienthost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStartJoinSessionValidation(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	_, err := StartJoinSession(context.Background(), dir, JoinOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnsureServerLuaAndPortFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureServerLua(dir); err != nil {
		t.Fatal(err)
	}
	port, err := ReserveLuaPort()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteLuaPortFile(dir, port); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "lua_server_port.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("empty port file")
	}
}
