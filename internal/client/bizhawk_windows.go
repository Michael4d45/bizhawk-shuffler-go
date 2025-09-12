//go:build windows

package client

import (
	"log"
	"os"
	"os/exec"

	"golang.org/x/sys/windows/registry"
)

func (c *BizHawkController) checkAndInstallVCRedist() {
	log.Printf("Checking if VC++ redistributables are already installed...")
	// Check if VC++ redist is already installed by checking registry
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\VisualStudio\14.0\VC\Runtimes\x64`, registry.QUERY_VALUE)
	if err == nil {
		_ = k.Close()
		log.Printf("VC++ redistributables already installed, skipping download and installation")
	} else {
		log.Printf("VC++ redistributables not found (registry check failed: %v), downloading and installing", err)
		// Download and install
		url := "https://aka.ms/vs/17/release/vc_redist.x64.exe"
		vcPath := "vc_redist.x64.exe"
		log.Printf("Downloading VC++ redistributable from %s to %s", url, vcPath)
		if err := c.DownloadFile(url, vcPath); err != nil {
			log.Printf("Failed to download VC++ redistributable: %v", err)
		}
		defer func() { _ = os.Remove(vcPath) }()
		log.Printf("Installing VC++ redistributable...")
		cmd := exec.Command(vcPath, "/quiet", "/norestart")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Printf("Failed to install VC++ redistributable: %v", err)
		}
		log.Printf("VC++ redistributable installed successfully")
	}
}
