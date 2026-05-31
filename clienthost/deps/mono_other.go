//go:build !linux

package deps

// IsMonoInstalled is always true off Linux (BizHawk uses EmuHawkMono.sh only on Linux).
func IsMonoInstalled() bool {
	return true
}

// MonoInstallHint is empty off Linux.
func MonoInstallHint() string {
	return ""
}

// InstallMono is a no-op off Linux.
func InstallMono(progress func(string)) error {
	return nil
}
