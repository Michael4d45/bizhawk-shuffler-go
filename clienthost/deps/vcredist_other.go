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
	return nil
}

// IsVCRedistInstalled is always true on non-Windows.
func (v *VCRedistInstaller) IsVCRedistInstalled() bool {
	return true
}

// InstallVCRedist is a no-op on non-Windows.
func (v *VCRedistInstaller) InstallVCRedist(progress func(string)) error {
	return nil
}
