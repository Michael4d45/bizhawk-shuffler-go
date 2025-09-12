//go:build windows

package client

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
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
	logPath := filepath.Join(os.TempDir(), "vc_redist_install.log")
	log.Printf("[DEBUG] installVCRedist: Running command: %s /quiet /norestart /log %s", vcPath, logPath)

	cmd := exec.Command(vcPath, "/quiet", "/norestart", "/log", logPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("[DEBUG] installVCRedist: Starting installation process...")
	log.Println("installVCRedist: This may take a few moments...")
	if err := cmd.Run(); err != nil {
		code := -1
		if ps := cmd.ProcessState; ps != nil {
			code = ps.ExitCode()
		}
		switch code {
		case 3010:
			log.Printf("[INFO] installVCRedist: VC++ redistributable installed successfully; reboot required (exit code 3010)")
		default:
			log.Printf("[ERROR] installVCRedist: Installer failed, exit code: %d, err: %v", code, err)
		}
	} else {
		log.Printf("[INFO] installVCRedist: VC++ redistributable installed successfully")
	}
	if isVCRedistPresentRegistry() || checkVCRedistInstalled() {
		log.Printf("[INFO] installVCRedist: Verified VC++ presence after install")
	} else {
		log.Printf("[ERROR] installVCRedist: VC++ still not detected; see log: %s", logPath)
	}
	log.Println("installVCRedist: Installation process complete.")
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

func isVCRedistPresentRegistry() bool {
	// Check registry for VC++ 2015-2022 redistributable
	// This is more reliable than file presence check
	keyPaths := []string{
		`SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\x64`,
		`SOFTWARE\WOW6432Node\Microsoft\VisualStudio\14.0\VC\Runtimes\x64`,
	}

	for _, keyPath := range keyPaths {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		defer func() { _ = key.Close() }()

		// Check if Installed value is 1
		installed, _, err := key.GetIntegerValue("Installed")
		if err == nil && installed == 1 {
			log.Printf("[DEBUG] isVCRedistPresentRegistry: Found VC++ redistributable in registry at %s", keyPath)
			return true
		}
	}
	log.Printf("[DEBUG] isVCRedistPresentRegistry: VC++ redistributable not found in registry")
	return false
}
