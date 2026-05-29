package clienthost

import (
	"fmt"
	"os"
	"runtime"

	"github.com/michael4d45/bizshuffle/clienthost/deps"
	"github.com/michael4d45/bizshuffle/clienthost/installer"
)

// DependencyID identifies an installable dependency.
type DependencyID string

const (
	DependencyBizHawk  DependencyID = "bizhawk"
	DependencyVCRedist DependencyID = "vcredist"
)

// DependencyItem is one row in the dependencies panel.
type DependencyItem struct {
	ID          DependencyID
	Label       string
	Status      string
	Detail      string
	ActionLabel string
}

// DependenciesSnapshot is the desktop dependency gate state.
type DependenciesSnapshot struct {
	Items       []DependencyItem
	PlayBlocked bool
}

// GetDependenciesSnapshot returns items that need user action and whether Join is blocked.
func GetDependenciesSnapshot(dataDir string) DependenciesSnapshot {
	var items []DependencyItem

	bh := GetBizHawkStatus(dataDir)
	if bh.Missing {
		items = append(items, DependencyItem{
			ID:          DependencyBizHawk,
			Label:       "BizHawk",
			Status:      "missing",
			Detail:      fmt.Sprintf("Not found — %s required", SupportedBizHawkVersion),
			ActionLabel: fmt.Sprintf("Install BizHawk %s", SupportedBizHawkVersion),
		})
	} else if bh.NeedsUpdate {
		installed := bh.InstalledVersion
		if installed == "" {
			installed = "unknown"
		}
		items = append(items, DependencyItem{
			ID:          DependencyBizHawk,
			Label:       "BizHawk",
			Status:      "outdated",
			Detail:      fmt.Sprintf("v%s installed — v%s or newer required", installed, SupportedBizHawkVersion),
			ActionLabel: fmt.Sprintf("Update to %s", SupportedBizHawkVersion),
		})
	}

	if runtime.GOOS == "windows" {
		vc := deps.NewVCRedistInstaller()
		if !vc.IsVCRedistInstalled() {
			items = append(items, DependencyItem{
				ID:          DependencyVCRedist,
				Label:       "Visual C++ runtime",
				Status:      "missing",
				Detail:      "Required for BizHawk on Windows",
				ActionLabel: "Install VC++ runtime",
			})
		}
	}

	return DependenciesSnapshot{Items: items, PlayBlocked: len(items) > 0}
}

// PlayBlockedMessage returns a user-facing message when Join is disabled.
func PlayBlockedMessage(snap DependenciesSnapshot) string {
	for _, item := range snap.Items {
		if item.ID == DependencyBizHawk && item.Status == "outdated" {
			return fmt.Sprintf("Update BizHawk using the button above (%s).", item.Detail)
		}
		if item.ID == DependencyBizHawk && item.Status == "missing" {
			return "Install BizHawk using the button above before joining."
		}
		if item.ID == DependencyVCRedist {
			return "Install the Visual C++ runtime using the button above before joining."
		}
	}
	return "Resolve dependencies above before joining."
}

// AssertPlayReady returns an error if dependencies block Join.
func AssertPlayReady(dataDir string) error {
	snap := GetDependenciesSnapshot(dataDir)
	if snap.PlayBlocked {
		return fmt.Errorf("%s", PlayBlockedMessage(snap))
	}
	return nil
}

// InstallDependency installs BizHawk or VC++ under the managed layout.
func InstallDependency(dataDir string, id DependencyID, progress func(string)) error {
	switch id {
	case DependencyBizHawk:
		return installBizHawkManaged(dataDir, progress)
	case DependencyVCRedist:
		return deps.NewVCRedistInstaller().InstallVCRedist(progress)
	default:
		return fmt.Errorf("unknown dependency %q", id)
	}
}

// InstallAllDependencies installs every dependency that still needs action.
func InstallAllDependencies(dataDir string, progress func(string)) error {
	snap := GetDependenciesSnapshot(dataDir)
	if len(snap.Items) == 0 {
		return nil
	}
	var firstErr error
	for _, item := range snap.Items {
		if err := InstallDependency(dataDir, item.ID, progress); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func installBizHawkManaged(dataDir string, progress func(string)) error {
	installDir := BizHawkInstallDir(dataDir)
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(installDir); err == nil {
		entries, _ := os.ReadDir(installDir)
		if len(entries) > 0 {
			if progress != nil {
				progress("Removing previous BizHawk install…")
			}
			if err := os.RemoveAll(installDir); err != nil {
				return err
			}
			if err := os.MkdirAll(installDir, 0o755); err != nil {
				return err
			}
		}
	}
	url, err := installer.GetBizHawkDownloadURLForVersion(SupportedBizHawkVersion)
	if err != nil {
		return err
	}
	bh := deps.NewBizHawkInstaller()
	if err := bh.InstallBizHawk(url, installDir, progress); err != nil {
		return err
	}
	_, err = ResolveEmuHawkPath(dataDir)
	return err
}
