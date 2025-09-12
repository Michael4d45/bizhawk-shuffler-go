//go:build windows

package client

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func (c *BizHawkController) checkAndInstallVCRedist() {
	log.Printf("[DEBUG] checkAndInstallVCRedist: Starting VC++ redistributable check")

	// Check for VC++ redist by looking for the installed files
	// This is a simpler approach that doesn't require registry access
	possiblePaths := []string{
		`C:\Windows\System32\vcruntime140.dll`,
		`C:\Windows\SysWOW64\vcruntime140.dll`,
	}

	log.Printf("[DEBUG] checkAndInstallVCRedist: Checking %d possible VC++ redistributable paths", len(possiblePaths))

	vcInstalled := false
	for i, path := range possiblePaths {
		log.Printf("[DEBUG] checkAndInstallVCRedist: Checking path %d/%d: %s", i+1, len(possiblePaths), path)
		if _, err := os.Stat(path); err == nil {
			log.Printf("[DEBUG] checkAndInstallVCRedist: Found VC++ redistributable at: %s", path)
			vcInstalled = true
			break
		} else {
			log.Printf("[DEBUG] checkAndInstallVCRedist: Path not found or not accessible: %s (error: %v)", path, err)
		}
	}

	if vcInstalled {
		log.Printf("[INFO] checkAndInstallVCRedist: VC++ redistributables appear to be installed (found runtime DLLs)")
		return
	}

	log.Printf("[INFO] checkAndInstallVCRedist: VC++ redistributables not found, proceeding with download and installation")
	c.installVCRedist()
}

func (c *BizHawkController) installVCRedist() {
	log.Printf("[DEBUG] installVCRedist: Starting VC++ redistributable installation process")

	// Download and install
	url := "https://aka.ms/vs/17/release/vc_redist.x64.exe"
	tempDir := os.TempDir()
	vcPath := filepath.Join(tempDir, "vc_redist.x64.exe")

	log.Printf("[DEBUG] installVCRedist: Temporary directory: %s", tempDir)
	log.Printf("[INFO] installVCRedist: Downloading VC++ redistributable from %s to %s", url, vcPath)

	// Check if temp directory is accessible
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		log.Printf("[ERROR] installVCRedist: Temporary directory does not exist: %s", tempDir)
		return
	}

	if err := c.DownloadFile(url, vcPath); err != nil {
		log.Printf("[ERROR] installVCRedist: Failed to download VC++ redistributable: %v", err)
		return
	}

	// Verify the downloaded file
	if info, err := os.Stat(vcPath); err != nil {
		log.Printf("[ERROR] installVCRedist: Downloaded file not accessible: %v", err)
	} else {
		log.Printf("[DEBUG] installVCRedist: Downloaded file size: %d bytes", info.Size())
	}

	defer func() {
		log.Printf("[DEBUG] installVCRedist: Cleaning up downloaded file: %s", vcPath)
		if err := os.Remove(vcPath); err != nil {
			log.Printf("[WARN] installVCRedist: Failed to clean up file %s: %v", vcPath, err)
		}
	}()

	log.Printf("[INFO] installVCRedist: Installing VC++ redistributable...")
	log.Printf("[DEBUG] installVCRedist: Running command: %s /quiet /norestart", vcPath)

	cmd := exec.Command(vcPath, "/quiet", "/norestart")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("[DEBUG] installVCRedist: Starting installation process...")
	fmt.Println("installVCRedist: This may take a few moments...")
	if err := cmd.Run(); err != nil {
		log.Printf("[ERROR] installVCRedist: Failed to install VC++ redistributable: %v", err)
		log.Printf("[DEBUG] installVCRedist: Command exit code: %d", cmd.ProcessState.ExitCode())
	} else {
		log.Printf("[INFO] installVCRedist: VC++ redistributable installed successfully")
		log.Printf("[DEBUG] installVCRedist: Installation completed without errors")
	}
	fmt.Println("installVCRedist: Installation process complete.")
}
