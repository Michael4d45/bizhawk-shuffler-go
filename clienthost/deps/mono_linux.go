//go:build linux

package deps

import (
	"fmt"
	"os"
	"os/exec"
)

// IsMonoInstalled reports whether the mono(1) launcher is on PATH (required by EmuHawkMono.sh).
func IsMonoInstalled() bool {
	_, err := exec.LookPath("mono")
	return err == nil
}

// MonoInstallHint returns a one-line install command for the detected distro, or a generic hint.
func MonoInstallHint() string {
	switch {
	case fileExists("/etc/arch-release"):
		return "Required for BizHawk. Install: sudo pacman -S mono"
	case fileExists("/etc/debian_version"):
		return "Required for BizHawk. Install: sudo apt install mono-complete"
	case fileExists("/etc/fedora-release"), fileExists("/etc/redhat-release"):
		return "Required for BizHawk. Install: sudo dnf install mono-complete"
	case fileExists("/etc/SuSE-release"), fileExists("/etc/SUSE-brand"):
		return "Required for BizHawk. Install: sudo zypper install mono-complete"
	default:
		return "Required for BizHawk. Install Mono from your distro packages (must be on PATH as mono)."
	}
}

// InstallMono rechecks PATH; Mono must be installed via the system package manager.
func InstallMono(progress func(string)) error {
	if progress != nil {
		progress("Checking for mono…")
	}
	if IsMonoInstalled() {
		return nil
	}
	return fmt.Errorf("%s", MonoInstallHint())
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
