//go:build linux

package clienthost

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/michael4d45/bizshuffle/assets"
)

// bizHawkLuaSocketDir returns {bizhawkRoot}/Lua/socket (BizHawk's luasocket native path).
func bizHawkLuaSocketDir(bizhawkRoot string) string {
	return filepath.Join(bizhawkRoot, "Lua", "socket")
}

// BizHawkLuaSocketInstalled reports whether socket.core is present for this BizHawk tree.
func BizHawkLuaSocketInstalled(bizhawkRoot string) bool {
	if bizhawkRoot == "" {
		return false
	}
	coreSO := filepath.Join(bizHawkLuaSocketDir(bizhawkRoot), "core.so")
	if st, err := os.Stat(coreSO); err == nil && st.Size() > 0 {
		return true
	}
	return false
}

// EnsureBizHawkLuaSocket installs socket.core.so into the BizHawk Lua tree when missing.
// Linux release zips ship Windows core.dll instead of core.so.
func EnsureBizHawkLuaSocket(bizhawkRoot string, progress func(string)) error {
	if bizhawkRoot == "" {
		return fmt.Errorf("bizhawk root not set")
	}
	if BizHawkLuaSocketInstalled(bizhawkRoot) {
		return nil
	}
	if runtime.GOARCH != "amd64" {
		return fmt.Errorf("bundled LuaSocket is only available for linux/amd64 (got %s)", runtime.GOARCH)
	}
	if len(assets.LuaSocketCoreLinuxAMD64) == 0 {
		return fmt.Errorf("LuaSocket core binary not embedded in this build")
	}
	if progress != nil {
		progress("Installing LuaSocket for BizHawk…")
	}
	socketDir := bizHawkLuaSocketDir(bizhawkRoot)
	if err := os.MkdirAll(socketDir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(socketDir, "core.so")
	if err := os.WriteFile(dest, assets.LuaSocketCoreLinuxAMD64, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	// Windows artifact from upstream zip is useless on Linux and confuses diagnostics.
	_ = os.Remove(filepath.Join(socketDir, "core.dll"))
	return nil
}

// bizHawkRootFromDataDir resolves the BizHawk install directory containing EmuHawkMono.sh.
func bizHawkRootFromDataDir(dataDir string) (string, error) {
	exe, err := ResolveEmuHawkPath(dataDir)
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}
