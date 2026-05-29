package clienthost

import (
	"path/filepath"
	"testing"
)

func TestCompareBizHawkVersions(t *testing.T) {
	if CompareBizHawkVersions("2.9", "2.10") >= 0 {
		t.Fatal("expected 2.9 < 2.10")
	}
	if CompareBizHawkVersions("2.11.1", "2.11.1") != 0 {
		t.Fatal("expected equal")
	}
}

func TestIsManagedBizHawkPath(t *testing.T) {
	dataDir := filepath.Join("C:", "Users", "me", "BizShuffle")
	managed := filepath.Join(dataDir, "BizHawk", "EmuHawk.exe")
	external := `C:\Program Files\BizHawk\EmuHawk.exe`
	if !IsManagedBizHawkPath(dataDir, managed) {
		t.Fatal("expected managed path")
	}
	if IsManagedBizHawkPath(dataDir, external) {
		t.Fatal("expected external path to be unmanaged")
	}
}

func TestGetDependenciesSnapshotMissingBizHawk(t *testing.T) {
	dir := t.TempDir()
	snap := GetDependenciesSnapshot(dir)
	if !snap.PlayBlocked {
		t.Fatal("expected play blocked when BizHawk missing")
	}
	if len(snap.Items) == 0 || snap.Items[0].ID != DependencyBizHawk {
		t.Fatalf("unexpected items: %+v", snap.Items)
	}
}
