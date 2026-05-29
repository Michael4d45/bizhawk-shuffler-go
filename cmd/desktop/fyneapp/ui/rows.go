package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// InspectorRow is a compact label | detail | action row for dependencies.
type InspectorRow struct {
	fyne.CanvasObject
	label  *widget.Label
	detail *widget.Label
	action fyne.CanvasObject
}

// NewInspectorRow builds a standardized dependency-style row.
func NewInspectorRow(labelText, detailText string, action fyne.CanvasObject) *InspectorRow {
	label := widget.NewLabel(labelText)
	label.Alignment = fyne.TextAlignTrailing
	label.Truncation = fyne.TextTruncateClip
	labelPad := canvas.NewRectangle(color.Transparent)
	labelPad.SetMinSize(fyne.NewSize(LabelWidth, 1))
	labelCol := container.NewStack(labelPad, label)

	detail := widget.NewLabel(detailText)
	detail.Wrapping = fyne.TextWrapWord
	detail.Importance = widget.MediumImportance

	cols := []fyne.CanvasObject{labelCol, detail}
	if action != nil {
		cols = append(cols, action)
	}
	row := &InspectorRow{
		label:  label,
		detail: detail,
		action: action,
		CanvasObject: container.NewHBox(cols...),
	}
	return row
}

// SetDetail updates the detail column.
func (r *InspectorRow) SetDetail(text string) {
	r.detail.SetText(text)
}

// SetActionEnabled enables or disables the action widget when it is a button.
func (r *InspectorRow) SetActionEnabled(enabled bool) {
	if b, ok := r.action.(*widget.Button); ok {
		if enabled {
			b.Enable()
		} else {
			b.Disable()
		}
	}
}
