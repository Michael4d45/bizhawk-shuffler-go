package datadir

import (
	"os"
	"path/filepath"
)

// Default returns the standard user data directory (~/BizShuffle on Unix, %USERPROFILE%\BizShuffle on Windows).
func Default() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "BizShuffle"), nil
}
