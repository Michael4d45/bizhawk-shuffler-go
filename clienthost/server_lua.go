package clienthost

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed assets/server.lua
var embeddedServerLua []byte

// EnsureServerLua copies bundled server.lua into dataDir when missing or stale.
func EnsureServerLua(dataDir string) (string, error) {
	dest := filepath.Join(dataDir, "server.lua")
	if len(embeddedServerLua) > 0 {
		if needsCopyEmbedded(dest, embeddedServerLua) {
			if err := os.WriteFile(dest, embeddedServerLua, 0o644); err != nil {
				return "", err
			}
		}
		return dest, nil
	}
	src, err := resolveServerLuaSourceOnDisk()
	if err != nil {
		return "", fmt.Errorf("server.lua asset not found in bundle: %w", err)
	}
	if needsCopy(src, dest) {
		if err := copyFile(src, dest); err != nil {
			return "", err
		}
	}
	return dest, nil
}

func needsCopyEmbedded(dest string, data []byte) bool {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return true
	}
	existing, err := os.ReadFile(dest)
	if err != nil {
		return true
	}
	return !bytes.Equal(existing, data)
}

func resolveServerLuaSourceOnDisk() (string, error) {
	candidates := []string{
		filepath.Join("assets", "server.lua"),
		filepath.Join("..", "assets", "server.lua"),
		filepath.Join("..", "..", "assets", "server.lua"),
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "assets", "server.lua"),
			filepath.Join(exeDir, "..", "assets", "server.lua"),
			filepath.Join(exeDir, "clienthost", "assets", "server.lua"),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return filepath.Abs(c)
		}
	}
	return "", os.ErrNotExist
}

func needsCopy(src, dest string) bool {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return true
	}
	si, err := os.Stat(src)
	if err != nil {
		return true
	}
	di, err := os.Stat(dest)
	if err != nil {
		return true
	}
	return si.ModTime().After(di.ModTime())
}
