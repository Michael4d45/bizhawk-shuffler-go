package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// NewTheme returns the BizShuffle desktop theme (muted surfaces, restrained accent).
func NewTheme() fyne.Theme {
	return &bizTheme{base: theme.DefaultTheme()}
}

type bizTheme struct {
	base fyne.Theme
}

func (t *bizTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		c := t.base.Color(name, variant)
		return blend(c, t.base.Color(theme.ColorNameForeground, variant), 0.03)
	case theme.ColorNameInputBackground:
		return SurfaceMuted()
	case theme.ColorNameSeparator:
		return BorderSubtle()
	}
	return t.base.Color(name, variant)
}

func (t *bizTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.base.Font(style)
}

func (t *bizTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t *bizTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return SpaceStandard
	case theme.SizeNameInnerPadding:
		return SpaceTight
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return t.base.Size(name)
}
