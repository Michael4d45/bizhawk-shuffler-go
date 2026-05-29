package ui

import (
	"image/color"

	"fyne.io/fyne/v2/theme"
)

// SurfaceMuted is a panel background tone above the window background.
func SurfaceMuted() color.Color {
	return blend(theme.Color(theme.ColorNameBackground), theme.Color(theme.ColorNameForeground), 0.06)
}

// BorderSubtle is a 1px-equivalent panel edge contrast.
func BorderSubtle() color.Color {
	return blend(theme.Color(theme.ColorNameBackground), theme.Color(theme.ColorNameForeground), 0.14)
}

func blend(a, b color.Color, t float64) color.Color {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	af := float64(ar) / 0xffff
	bf := float64(t)
	return color.NRGBA{
		R: uint8((af*(1-bf) + float64(br)/0xffff*bf) * 255),
		G: uint8((float64(ag)/0xffff*(1-bf) + float64(bg)/0xffff*bf) * 255),
		B: uint8((float64(ab)/0xffff*(1-bf) + float64(bb)/0xffff*bf) * 255),
		A: uint8(float64(aa) / 0xffff * 255),
	}
}
