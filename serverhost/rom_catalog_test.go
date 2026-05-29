package serverhost

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListRomsEmpty(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if files := ListRoms(); len(files) != 0 {
		t.Fatalf("expected empty, got %v", files)
	}
}

func TestSyncCatalogFromRoms(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(dir, "roms"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "roms", "game.nes"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New()
	t.Cleanup(func() { _ = s.StopBroadcaster() })
	updated, err := s.SyncCatalogFromRoms()
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal(" expected catalog update")
	}
	st := s.SnapshotState()
	if len(st.MainGames) != 1 || st.MainGames[0].File != "game.nes" {
		t.Fatalf("got %+v", st.MainGames)
	}
}
