package clienthost

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// BizHawkInstallDir returns the managed BizHawk root: {dataDir}/BizHawk.
func BizHawkInstallDir(dataDir string) string {
	return filepath.Join(dataDir, "BizHawk")
}

// IsManagedBizHawkPath reports whether exePath is under dataDir/BizHawk.
func IsManagedBizHawkPath(dataDir, exePath string) bool {
	root, err := filepath.Abs(BizHawkInstallDir(dataDir))
	if err != nil {
		return false
	}
	normalized, err := filepath.Abs(exePath)
	if err != nil {
		return false
	}
	if normalized == root {
		return true
	}
	rel, err := filepath.Rel(root, normalized)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..")
}

func clearStaleBizhawkConfig(dataDir string, cfg Config) {
	path := strings.TrimSpace(cfg["bizhawk_path"])
	if path == "" {
		return
	}
	if !IsManagedBizHawkPath(dataDir, path) {
		delete(cfg, "bizhawk_path")
		_ = SaveConfig(dataDir, cfg)
	}
}

func findEmuHawkInDir(dir string) (string, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", fmt.Errorf("directory not found: %s", dir)
	}
	if runtime.GOOS == "windows" {
		direct := filepath.Join(dir, "EmuHawk.exe")
		if _, err := os.Stat(direct); err == nil {
			return direct, nil
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			nested := filepath.Join(dir, e.Name(), "EmuHawk.exe")
			if _, err := os.Stat(nested); err == nil {
				return nested, nil
			}
		}
		return "", fmt.Errorf("EmuHawk.exe not found under %s", dir)
	}
	direct := filepath.Join(dir, "EmuHawkMono.sh")
	if _, err := os.Stat(direct); err == nil {
		return direct, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		nested := filepath.Join(dir, e.Name(), "EmuHawkMono.sh")
		if _, err := os.Stat(nested); err == nil {
			return nested, nil
		}
	}
	return "", fmt.Errorf("EmuHawk launcher not found under %s", dir)
}

func persistBizhawkPath(dataDir string, cfg Config, absPath string) (string, error) {
	if cfg["bizhawk_path"] != absPath {
		cfg["bizhawk_path"] = absPath
		if err := SaveConfig(dataDir, cfg); err != nil {
			return "", err
		}
	}
	return absPath, nil
}

// ResolveEmuHawkPath locates EmuHawk only under {dataDir}/BizHawk.
func ResolveEmuHawkPath(dataDir string) (string, error) {
	cfg, err := LoadConfig(dataDir)
	if err != nil {
		return "", err
	}
	if err := cfg.EnsureDefaults(); err != nil {
		return "", err
	}
	clearStaleBizhawkConfig(dataDir, cfg)
	exe, err := findEmuHawkInDir(BizHawkInstallDir(dataDir))
	if err != nil {
		return "", fmt.Errorf("BizHawk not found under %s: %w", BizHawkInstallDir(dataDir), err)
	}
	return persistBizhawkPath(dataDir, cfg, exe)
}

// ResolveInstalledBizHawkVersion returns detected version or supported when managed.
func ResolveInstalledBizHawkVersion(dataDir, exePath string) string {
	if v := DetectInstalledBizHawkVersion(exePath); v != "" {
		return v
	}
	if IsManagedBizHawkPath(dataDir, exePath) {
		return SupportedBizHawkVersion
	}
	return ""
}

// GetBizHawkStatus reports managed BizHawk install state.
func GetBizHawkStatus(dataDir string) BizHawkStatus {
	supported := SupportedBizHawkVersion
	exe, err := ResolveEmuHawkPath(dataDir)
	if err != nil {
		return BizHawkStatus{
			SupportedVersion: supported,
			Missing:          true,
		}
	}
	installed := ResolveInstalledBizHawkVersion(dataDir, exe)
	return BizHawkStatus{
		ExePath:          exe,
		InstalledVersion: installed,
		SupportedVersion: supported,
		NeedsUpdate:      BizHawkNeedsUpdate(installed, supported),
	}
}

// DefaultDataDir returns the standard user data directory (~/BizShuffle).
func DefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "BizShuffle"), nil
}

// EnsureDataDirs creates roms, saves, plugins under dataDir.
func EnsureDataDirs(dataDir string) error {
	for _, sub := range []string{"roms", "saves", "plugins"} {
		if err := os.MkdirAll(filepath.Join(dataDir, sub), 0o755); err != nil {
			return err
		}
	}
	return nil
}
