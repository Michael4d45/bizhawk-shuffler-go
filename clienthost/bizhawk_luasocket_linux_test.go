//go:build linux

package clienthost

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/michael4d45/bizshuffle/assets"
)

func TestEnsureBizHawkLuaSocket(t *testing.T) {
	if len(assets.LuaSocketCoreLinuxAMD64) == 0 {
		t.Skip("LuaSocket core not embedded")
	}
	root := t.TempDir()
	if err := EnsureBizHawkLuaSocket(root, nil); err != nil {
		t.Fatal(err)
	}
	core := filepath.Join(root, "Lua", "socket", "core.so")
	if st, err := os.Stat(core); err != nil || st.Size() == 0 {
		t.Fatalf("core.so missing: %v", err)
	}
	if !BizHawkLuaSocketInstalled(root) {
		t.Fatal("expected installed after ensure")
	}
	if err := EnsureBizHawkLuaSocket(root, nil); err != nil {
		t.Fatal("second ensure:", err)
	}
}
