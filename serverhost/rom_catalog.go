package serverhost

import (
	"os"
	"path/filepath"

	"github.com/michael4d45/bizshuffle/protocol"
)

// ListRoms returns relative paths of files under ./roms (forward slashes).
func ListRoms() []string {
	romsDir := "./roms"
	if _, err := os.Stat(romsDir); err != nil {
		return nil
	}
	var files []string
	_ = filepath.Walk(romsDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(romsDir, p)
		if err != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files
}

func gameEntryHasFile(entries []protocol.GameEntry, file string) bool {
	for _, g := range entries {
		if g.File == file {
			return true
		}
		for _, ex := range g.ExtraFiles {
			if ex == file {
				return true
			}
		}
	}
	return false
}

// SyncCatalogFromRoms merges ROM files from ./roms into MainGames and runs mode setup when needed.
// Returns true when catalog state was updated or setup ran.
func (s *Server) SyncCatalogFromRoms() (bool, error) {
	files := ListRoms()
	if len(files) == 0 {
		return false, nil
	}

	var merged bool
	s.UpdateStateAndPersist(func(st *protocol.ServerState) {
		main := append([]protocol.GameEntry(nil), st.MainGames...)
		for _, f := range files {
			if !gameEntryHasFile(main, f) {
				main = append(main, protocol.GameEntry{File: f})
				merged = true
			}
		}
		st.MainGames = main
	})

	st := s.SnapshotState()
	enabled := make(map[string]bool)
	for _, g := range st.Games {
		enabled[g] = true
	}
	needsSetup := merged || len(enabled) == 0
	if !needsSetup {
		for _, f := range files {
			if !enabled[f] {
				needsSetup = true
				break
			}
		}
	}
	if !needsSetup && len(st.MainGames) == 0 {
		needsSetup = true
	}
	if !needsSetup {
		return false, nil
	}
	if err := s.GetGameModeHandler().SetupState(); err != nil {
		return merged, err
	}
	s.broadcastGamesUpdate(nil)
	return true, nil
}
