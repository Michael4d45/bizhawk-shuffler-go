//go:build windows

package deps

import (
	"github.com/michael4d45/bizshuffle/internal/installer"
)

// VCRedistInstaller wraps the installer's VCRedistInstaller
type VCRedistInstaller struct {
	impl *installer.VCRedistInstaller
}

// NewVCRedistInstaller creates a new VC++ redistributable installer
func NewVCRedistInstaller() *VCRedistInstaller {
	return &VCRedistInstaller{
		impl: installer.NewVCRedistInstaller(),
	}
}

// CheckAndInstallVCRedist checks if VC++ redistributables are installed and installs if needed
func (v *VCRedistInstaller) CheckAndInstallVCRedist(progress func(msg string)) error {
	return v.impl.CheckAndInstallVCRedist(progress)
}
