//go:build linux

package deps

import (
	"strings"
	"testing"
)

func TestMonoInstallHintArch(t *testing.T) {
	if !fileExists("/etc/arch-release") {
		t.Skip("not on Arch")
	}
	hint := MonoInstallHint()
	if !strings.Contains(hint, "pacman") {
		t.Fatalf("expected pacman hint on Arch, got %q", hint)
	}
}

func TestMonoInstallHintGeneric(t *testing.T) {
	if fileExists("/etc/arch-release") || fileExists("/etc/debian_version") {
		t.Skip("distro-specific test only for unknown distros")
	}
	hint := MonoInstallHint()
	if !strings.Contains(hint, "PATH") {
		t.Fatalf("expected generic PATH hint, got %q", hint)
	}
}
