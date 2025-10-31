package deps

import (
	"github.com/michael4d45/bizshuffle/internal/installer"
)

// BizHawkInstaller wraps the installer's BizHawkInstaller
type BizHawkInstaller struct {
	impl *installer.BizHawkInstaller
}

// NewBizHawkInstaller creates a new BizHawk installer
func NewBizHawkInstaller() *BizHawkInstaller {
	return &BizHawkInstaller{
		impl: installer.NewBizHawkInstaller(),
	}
}

// InstallBizHawk downloads and installs BizHawk to the specified directory
func (b *BizHawkInstaller) InstallBizHawk(downloadURL, installDir string, progress func(msg string)) error {
	return b.impl.InstallBizHawk(downloadURL, installDir, progress)
}

// GetBizHawkDownloadURL returns the default BizHawk download URL for the current platform
func GetBizHawkDownloadURL() string {
	return installer.GetBizHawkDownloadURL()
}
