//go:build !windows

package deps

// VCRedistInstaller is a no-op on non-Windows platforms
type VCRedistInstaller struct{}

// NewVCRedistInstaller creates a new VC++ redistributable installer (no-op on non-Windows)
func NewVCRedistInstaller() *VCRedistInstaller {
	return &VCRedistInstaller{}
}

// CheckAndInstallVCRedist is a no-op on non-Windows platforms
func (v *VCRedistInstaller) CheckAndInstallVCRedist(progress func(msg string)) error {
	// VC++ redistributable is only needed on Windows
	return nil
}

