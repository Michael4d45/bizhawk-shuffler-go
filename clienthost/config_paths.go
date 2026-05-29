package clienthost

import "path/filepath"

func configPath(dataDir string) string {
	return filepath.Join(dataDir, "config.json")
}
