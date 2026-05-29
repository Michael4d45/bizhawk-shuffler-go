package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// SectionPanel is a surfaced region with title, optional subtitle, header actions, body, and footer.
type SectionPanel struct {
	Root      fyne.CanvasObject
	bodyBox   *fyne.Container
	footerBox *fyne.Container
}

// NewPanel wraps content in a muted surface with padding.
func NewPanel(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(SurfaceMuted())
	bg.StrokeColor = BorderSubtle()
	bg.StrokeWidth = 1
	bg.CornerRadius = 4
	return container.NewStack(
		bg,
		container.NewPadded(content),
	)
}

// NewSectionPanel builds a section with standard panel anatomy.
func NewSectionPanel(title, subtitle string, actions, body, footer fyne.CanvasObject) *SectionPanel {
	titleObj := NewSectionTitle(title)
	headerLeft := container.NewVBox(titleObj)
	if subtitle != "" {
		headerLeft.Add(NewMuted(subtitle))
	}
	// Title/subtitle in center so they get horizontal space; actions on the right.
	var header fyne.CanvasObject
	if actions != nil {
		header = container.NewBorder(nil, nil, nil, actions, headerLeft)
	} else {
		header = headerLeft
	}

	bodyBox := container.NewVBox()
	if body != nil {
		bodyBox.Add(body)
	}
	footerBox := container.NewVBox()
	if footer != nil {
		footerBox.Add(footer)
	}

	inner := container.NewVBox(
		header,
		container.NewPadded(bodyBox),
	)
	if footer != nil {
		inner.Add(container.NewPadded(footerBox))
	}

	sp := &SectionPanel{
		Root:      NewPanel(inner),
		bodyBox:   bodyBox,
		footerBox: footerBox,
	}
	return sp
}

// SetBody replaces the panel body content.
func (s *SectionPanel) SetBody(content fyne.CanvasObject) {
	s.bodyBox.Objects = nil
	if content != nil {
		s.bodyBox.Add(content)
	}
	s.bodyBox.Refresh()
}

// SetFooter replaces the panel footer strip.
func (s *SectionPanel) SetFooter(content fyne.CanvasObject) {
	s.footerBox.Objects = nil
	if content != nil {
		s.footerBox.Add(content)
	}
	s.footerBox.Refresh()
}

// NewScrollBody wraps page content in a vertical scroll region.
func NewScrollBody(content fyne.CanvasObject) fyne.CanvasObject {
	return container.NewScroll(content)
}

// SetPageSections replaces scroll page content with spaced section panels.
func SetPageSections(box *fyne.Container, sections ...fyne.CanvasObject) {
	box.Objects = nil
	for i, s := range sections {
		if i > 0 {
			spacer := canvas.NewRectangle(color.Transparent)
			spacer.SetMinSize(fyne.NewSize(0, SpaceSection))
			box.Add(spacer)
		}
		box.Add(s)
	}
	box.Refresh()
}

// NewPageVBox stacks section panels with relaxed sectional spacing.
func NewPageVBox(sections ...fyne.CanvasObject) fyne.CanvasObject {
	box := container.NewVBox()
	SetPageSections(box, sections...)
	return container.NewPadded(box)
}

// NewHeaderSurface is the fixed window header (title + optional right content).
func NewHeaderSurface(title string, right fyne.CanvasObject) fyne.CanvasObject {
	left := NewTitle(title)
	if right == nil {
		return container.NewPadded(left)
	}
	return container.NewPadded(container.NewBorder(nil, nil, left, right, nil))
}

// NewFooterRow places left and right utility controls on one line.
func NewFooterRow(left, right fyne.CanvasObject) fyne.CanvasObject {
	return container.NewPadded(container.NewBorder(nil, nil, left, right, nil))
}

// NewActionBar is a tight row for primary/secondary actions below a form.
func NewActionBar(objects ...fyne.CanvasObject) fyne.CanvasObject {
	return container.NewHBox(objects...)
}
