//go:build windows && cgo
// +build windows,cgo

package client

import "github.com/go-vgo/robotgo"

// keyTap calls robotgo.KeyTap on Windows
func keyTap(key string, modifiers ...string) error {
	// Convert []string to []any for robotgo.KeyTap
	mods := make([]any, len(modifiers))
	for i, m := range modifiers {
		mods[i] = m
	}
	return robotgo.KeyTap(key, mods...)
}
