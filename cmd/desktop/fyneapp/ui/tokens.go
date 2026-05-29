package ui

import "fyne.io/fyne/v2"

// Spacing scale (density zones).
const (
	SpaceTight    float32 = 8
	SpaceStandard float32 = 12
	SpaceSection  float32 = 24
)

// Layout constants.
const (
	LabelWidth     float32 = 120
	WindowDefaultW float32 = 900
	WindowDefaultH float32 = 640
)

// WindowDefaultSize returns the default shell window size.
func WindowDefaultSize() fyne.Size {
	return fyne.NewSize(WindowDefaultW, WindowDefaultH)
}
