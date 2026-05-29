package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// StatusSeverity styles global status feedback.
type StatusSeverity int

const (
	StatusSeverityInfo StatusSeverity = iota
	StatusSeveritySuccess
	StatusSeverityWarning
	StatusSeverityError
)

// NewTitle is the app header label.
func NewTitle(text string) fyne.CanvasObject {
	l := widget.NewLabel(text)
	l.TextStyle = fyne.TextStyle{Bold: true}
	return l
}

// NewSectionTitle is a panel heading.
func NewSectionTitle(text string) fyne.CanvasObject {
	l := widget.NewLabel(text)
	l.TextStyle = fyne.TextStyle{Bold: true}
	return l
}

// NewMuted returns secondary metadata copy.
func NewMuted(text string) fyne.CanvasObject {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	return l
}

// NewStatus returns a status line with semantic color.
func NewStatus(text string, sev StatusSeverity) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	switch sev {
	case StatusSeveritySuccess:
		l.Importance = widget.SuccessImportance
	case StatusSeverityWarning:
		l.Importance = widget.WarningImportance
	case StatusSeverityError:
		l.Importance = widget.DangerImportance
	default:
		l.Importance = widget.MediumImportance
	}
	return l
}

// SetStatus updates label text and severity styling.
func SetStatus(l *widget.Label, text string, sev StatusSeverity) {
	l.SetText(text)
	switch sev {
	case StatusSeveritySuccess:
		l.Importance = widget.SuccessImportance
	case StatusSeverityWarning:
		l.Importance = widget.WarningImportance
	case StatusSeverityError:
		l.Importance = widget.DangerImportance
	default:
		l.Importance = widget.MediumImportance
	}
	l.Refresh()
}
