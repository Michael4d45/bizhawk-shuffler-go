package ui

import "fyne.io/fyne/v2"

// Spacing scale (density zones).
const (
	SpaceMicro    float32 = 4
	SpaceTight    float32 = 8
	SpaceStandard float32 = 12
	SpaceRelaxed  float32 = 16
	SpaceSection  float32 = 24
	SpacePage     float32 = 32
)

// Layout constants.
const (
	LabelWidth       float32 = 120
	DiscoveryListMin float32 = 140
	WindowMinW       float32 = 720
	WindowMinH       float32 = 520
	WindowDefaultW   float32 = 900
	WindowDefaultH   float32 = 640
)

// WindowMinSize returns the minimum shell window size.
func WindowMinSize() fyne.Size {
	return fyne.NewSize(WindowMinW, WindowMinH)
}

// WindowDefaultSize returns the default shell window size.
func WindowDefaultSize() fyne.Size {
	return fyne.NewSize(WindowDefaultW, WindowDefaultH)
}
