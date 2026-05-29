package clienthost

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/michael4d45/bizshuffle/assets"
)

// EnsureServerLua copies bundled server.lua into dataDir when missing or stale.
func EnsureServerLua(dataDir string) (string, error) {
	if len(assets.ServerLua) == 0 {
		return "", fmt.Errorf("server.lua asset is empty")
	}
	dest := filepath.Join(dataDir, "server.lua")
	if needsCopyEmbedded(dest, assets.ServerLua) {
		if err := os.WriteFile(dest, assets.ServerLua, 0o644); err != nil {
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
