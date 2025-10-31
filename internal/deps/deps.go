package deps

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

// ProgressCallback is called with progress messages during dependency installation
type ProgressCallback func(msg string)

// DependencyManager manages checking and installation of client dependencies
type DependencyManager struct {
	bizhawkInstaller   *BizHawkInstaller
	vcRedistInstaller  *VCRedistInstaller
	progressCallback   ProgressCallback
	bizhawkInstallDir  string
	defaultBizhawkPath string
}

// InstallPromptCallback is called to ask the user if they want to install a missing dependency
// Returns true if the user wants to install, false otherwise
type InstallPromptCallback func(dependencyName string) bool

// NewDependencyManager creates a new dependency manager
// bizhawkInstallDir is the directory where BizHawk should be installed if missing
// configuredPath is an optional configured BizHawk executable path that should be checked first
func NewDependencyManager(bizhawkInstallDir string, progressCallback ProgressCallback) *DependencyManager {
	return NewDependencyManagerWithPath(bizhawkInstallDir, "", progressCallback)
}

// NewDependencyManagerWithPath creates a new dependency manager with a specific configured path
func NewDependencyManagerWithPath(bizhawkInstallDir, configuredPath string, progressCallback ProgressCallback) *DependencyManager {
	dm := &DependencyManager{
		progressCallback:  progressCallback,
		bizhawkInstallDir: bizhawkInstallDir,
	}
	
	// Use configured path as default if provided
	if configuredPath != "" {
		dm.defaultBizhawkPath = configuredPath
	} else if bizhawkInstallDir != "" {
		// Otherwise determine default BizHawk executable path from install dir
		if runtime.GOOS == "windows" {
			dm.defaultBizhawkPath = filepath.Join(bizhawkInstallDir, "EmuHawk.exe")
		} else {
			dm.defaultBizhawkPath = filepath.Join(bizhawkInstallDir, "EmuHawkMono.sh")
		}
	}
	
	dm.bizhawkInstaller = NewBizHawkInstaller()
	if runtime.GOOS == "windows" {
		dm.vcRedistInstaller = NewVCRedistInstaller()
	}
	
	return dm
}

// CheckAndInstallDependencies checks if all required dependencies are installed
// and installs them if missing. If promptCallback is provided, it will be called
// to ask the user before installing missing dependencies.
// Returns the BizHawk executable path if installed.
func (dm *DependencyManager) CheckAndInstallDependencies(promptCallback InstallPromptCallback) (string, error) {
	// Check and install VC++ redistributable on Windows
	if runtime.GOOS == "windows" && dm.vcRedistInstaller != nil {
		if dm.progressCallback != nil {
			dm.progressCallback("Checking VC++ redistributables...")
		}
		if err := dm.vcRedistInstaller.CheckAndInstallVCRedist(dm.progressCallback); err != nil {
			// Log but don't fail - VC++ redistributable is optional
			if dm.progressCallback != nil {
				dm.progressCallback(fmt.Sprintf("Warning: VC++ redistributable check failed: %v", err))
			}
			log.Printf("VC++ redistributable check failed: %v", err)
		}
	}
	
	// Check if BizHawk is installed
	// First check if default path exists
	bizhawkPath := ""
	if dm.defaultBizhawkPath != "" && dm.isBizHawkInstalled(dm.defaultBizhawkPath) {
		bizhawkPath = dm.defaultBizhawkPath
	}
	
	// If not found at default location, try to find BizHawk in common locations
	if bizhawkPath == "" {
		if found := dm.findBizHawk(); found != "" {
			bizhawkPath = found
		}
	}
	
	// If still not found, need to install it
	if bizhawkPath == "" {
		if dm.bizhawkInstallDir == "" {
			return "", fmt.Errorf("BizHawk not found and no install directory specified")
		}
		
		// Prompt user if callback provided
		if promptCallback != nil {
			if !promptCallback("BizHawk") {
				return "", fmt.Errorf("BizHawk installation cancelled by user")
			}
		}
		
		if dm.progressCallback != nil {
			dm.progressCallback("BizHawk not found, installing...")
		}
		
		// Install BizHawk
		bizhawkURL := GetBizHawkDownloadURL()
		if err := dm.bizhawkInstaller.InstallBizHawk(bizhawkURL, dm.bizhawkInstallDir, dm.progressCallback); err != nil {
			return "", fmt.Errorf("failed to install BizHawk: %w", err)
		}
		
		// Update path after installation
		if runtime.GOOS == "windows" {
			bizhawkPath = filepath.Join(dm.bizhawkInstallDir, "EmuHawk.exe")
		} else {
			bizhawkPath = filepath.Join(dm.bizhawkInstallDir, "EmuHawkMono.sh")
		}
	}
	
	// Verify BizHawk exists at the resolved path
	if !dm.isBizHawkInstalled(bizhawkPath) {
		return "", fmt.Errorf("BizHawk not found at %s", bizhawkPath)
	}
	
	return bizhawkPath, nil
}

// IsBizHawkInstalled checks if BizHawk is installed at the given path
func (dm *DependencyManager) IsBizHawkInstalled(path string) bool {
	return dm.isBizHawkInstalled(path)
}

func (dm *DependencyManager) isBizHawkInstalled(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// findBizHawk attempts to find BizHawk in common installation locations
func (dm *DependencyManager) findBizHawk() string {
	// Try relative to current executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		var candidate string
		if runtime.GOOS == "windows" {
			candidate = filepath.Join(exeDir, "BizHawk", "EmuHawk.exe")
		} else {
			candidate = filepath.Join(exeDir, "BizHawk", "EmuHawkMono.sh")
		}
		if dm.isBizHawkInstalled(candidate) {
			return candidate
		}
	}
	
	// Try current working directory
	if cwd, err := os.Getwd(); err == nil {
		var candidate string
		if runtime.GOOS == "windows" {
			candidate = filepath.Join(cwd, "BizHawk", "EmuHawk.exe")
		} else {
			candidate = filepath.Join(cwd, "BizHawk", "EmuHawkMono.sh")
		}
		if dm.isBizHawkInstalled(candidate) {
			return candidate
		}
	}
	
	return ""
}

