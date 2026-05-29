//go:build !windows || !cgo

package clienthost

import "fmt"

// keyTap is a no-op stub for non-Windows platforms
func keyTap(key string, modifiers ...string) error {
	return fmt.Errorf("fullscreen toggle is not supported on this platform")
}
