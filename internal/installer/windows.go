//go:build windows

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// VCRedistInstaller handles VC++ redistributable installation
type VCRedistInstaller struct {
	downloader *Downloader
}

// NewVCRedistInstaller creates a new VC++ redistributable installer
func NewVCRedistInstaller() *VCRedistInstaller {
	return &VCRedistInstaller{
		downloader: NewDownloader(),
	}
}

// CheckAndInstallVCRedist checks if VC++ redistributables are installed and installs if needed
func (v *VCRedistInstaller) CheckAndInstallVCRedist(progress func(msg string)) error {
	if progress != nil {
		progress("Checking VC++ redistributables...")
	}

	if v.IsVCRedistInstalled() {
		if progress != nil {
			progress("VC++ redistributables already installed")
		}
		return nil
	}

	if progress != nil {
		progress("VC++ redistributables not found, installing...")
	}
	return v.InstallVCRedist(progress)
}

// InstallVCRedist downloads and installs VC++ redistributables
func (v *VCRedistInstaller) InstallVCRedist(progress func(msg string)) error {
	url := "https://aka.ms/vs/17/release/vc_redist.x64.exe"
	tempDir := os.TempDir()
	vcPath := filepath.Join(tempDir, "vc_redist.x64.exe")

	if progress != nil {
		progress("Downloading VC++ redistributable...")
	}

	if err := v.downloader.DownloadFile(url, vcPath, nil); err != nil {
		return fmt.Errorf("failed to download VC++ redistributable: %w", err)
	}
	defer func() { _ = os.Remove(vcPath) }()

	if progress != nil {
		progress("Installing VC++ redistributable (this may take a moment)...")
	}

	logPath := filepath.Join(os.TempDir(), "vc_redist_install.log")
	cmd := exec.Command(vcPath, "/quiet", "/norestart", "/log", logPath)

	if err := cmd.Run(); err != nil {
		code := -1
		if ps := cmd.ProcessState; ps != nil {
			code = ps.ExitCode()
		}
		if code == 3010 {
			// Reboot required is not an error
			if progress != nil {
				progress("VC++ redistributable installed (reboot may be required)")
			}
			return nil
		}
		return fmt.Errorf("installer failed with exit code %d: %w", code, err)
	}

	if !v.IsVCRedistInstalled() {
		return fmt.Errorf("VC++ redistributable installation may have failed; see log: %s", logPath)
	}

	if progress != nil {
		progress("VC++ redistributable installed successfully")
	}
	return nil
}

// IsVCRedistInstalled checks if VC++ redistributables are installed
func (v *VCRedistInstaller) IsVCRedistInstalled() bool {
	// Check registry first (more reliable)
	if v.isVCRedistPresentRegistry() {
		return true
	}
	// Fallback to file check
	return v.checkVCRedistInstalled()
}

func (v *VCRedistInstaller) checkVCRedistInstalled() bool {
	possiblePaths := []string{
		`C:\Windows\System32\vcruntime140.dll`,
		`C:\Windows\SysWOW64\vcruntime140.dll`,
	}
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func (v *VCRedistInstaller) isVCRedistPresentRegistry() bool {
	keyPaths := []string{
		`SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\x64`,
		`SOFTWARE\WOW6432Node\Microsoft\VisualStudio\14.0\VC\Runtimes\x64`,
	}

	for _, keyPath := range keyPaths {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.QUERY_VALUE)
		if err != nil {
			continue
		}

		installed, _, err := key.GetIntegerValue("Installed")
		// explicitly close and ignore Close error
		_ = key.Close()
		if err == nil && installed == 1 {
			return true
		}
	}
	return false
}
