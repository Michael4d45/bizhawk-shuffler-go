//go:build windows

package client

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func (c *BizHawkController) checkAndInstallVCRedist() {
	log.Printf("[DEBUG] checkAndInstallVCRedist: Starting VC++ redistributable check")

	// Check for VC++ redist by looking for the installed files
	// This is a simpler approach that doesn't require registry access
	if checkVCRedistInstalled() {
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

	// Add timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, vcPath, "/quiet", "/norestart")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("[DEBUG] installVCRedist: Starting installation process with 20-second timeout...")
	fmt.Println("installVCRedist: This may take a few moments...")
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("[ERROR] installVCRedist: Installation timed out after 20 seconds")
			// Check again if VC++ is installed
			log.Printf("[INFO] installVCRedist: Verifying if VC++ redistributable is now installed after timeout...")
			if checkVCRedistInstalled() {
				return
			}
			log.Printf("[ERROR] installVCRedist: VC++ redistributable installation failed due to timeout")
		} else {
			log.Printf("[ERROR] installVCRedist: Failed to install VC++ redistributable: %v", err)
			log.Printf("[DEBUG] installVCRedist: Command exit code: %d", cmd.ProcessState.ExitCode())
		}
	} else {
		log.Printf("[INFO] installVCRedist: VC++ redistributable installed successfully")
		log.Printf("[DEBUG] installVCRedist: Installation completed without errors")
	}
	fmt.Println("installVCRedist: Installation process complete.")
}

func checkVCRedistInstalled() bool {
	possiblePaths := []string{
		`C:\Windows\System32\vcruntime140.dll`,
		`C:\Windows\SysWOW64\vcruntime140.dll`,
	}
	vcInstalled := false
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			log.Printf("[DEBUG] installVCRedist: Found VC++ redistributable at: %s", path)
			vcInstalled = true
			break
		}
	}
	if vcInstalled {
		log.Printf("[INFO] installVCRedist: VC++ redistributable appears to be installed despite timeout")
	} else {
		log.Printf("[ERROR] installVCRedist: VC++ redistributable still not found after installation attempt")
	}
	return vcInstalled
}
