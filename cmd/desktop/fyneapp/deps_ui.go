package fyneapp

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/michael4d45/bizshuffle/clienthost"
	"github.com/michael4d45/bizshuffle/cmd/desktop/fyneapp/ui"
)

func renderDepsPanel(
	w *shellWidgets,
	snap clienthost.DependenciesSnapshot,
	depsChecking bool,
	installing bool,
	onInstallAll func(),
	onInstallOne func(item clienthost.DependencyItem),
) {
	if depsChecking {
		progress := widget.NewProgressBarInfinite()
		progress.Start()
		w.depsPanel.SetBody(container.NewVBox(ui.NewMuted("Checking dependencies…"), progress))
		w.depsPanel.SetFooter(nil)
		return
	}

	var rows []fyne.CanvasObject
	if len(snap.Items) >= 2 && onInstallAll != nil {
		btn := widget.NewButton("Install all", onInstallAll)
		btn.Importance = widget.HighImportance
		if installing {
			btn.Disable()
		}
		rows = append(rows, btn)
	}
	for _, item := range snap.Items {
		it := item
		rows = append(rows, dependencyRow(it, installing, onInstallOne))
	}
	if len(rows) == 0 && snap.PlayBlocked {
		rows = append(rows, ui.NewMuted(clienthost.PlayBlockedMessage(snap)))
	}
	w.depsPanel.SetBody(container.NewVBox(rows...))

	if snap.PlayBlocked {
		w.depsPanel.SetFooter(ui.NewMuted(clienthost.PlayBlockedMessage(snap)))
	} else {
		w.depsPanel.SetFooter(nil)
	}
}

// dependencyRow is a stacked label, detail, and install action (readable on narrow layouts).
func dependencyRow(item clienthost.DependencyItem, installing bool, onInstallOne func(clienthost.DependencyItem)) fyne.CanvasObject {
	title := widget.NewLabel(item.Label)
	title.TextStyle = fyne.TextStyle{Bold: true}
	parts := []fyne.CanvasObject{title, ui.NewMuted(item.Detail)}
	if onInstallOne != nil {
		action := widget.NewButton(item.ActionLabel, func() { onInstallOne(item) })
		action.Importance = widget.HighImportance
		if installing {
			action.Disable()
		}
		parts = append(parts, action)
	}
	return container.NewVBox(parts...)
}

// depsPanelNeeded reports whether the dependencies section should appear.
func depsPanelNeeded(snap clienthost.DependenciesSnapshot, depsChecking bool) bool {
	if depsChecking {
		return true
	}
	if len(snap.Items) > 0 {
		return true
	}
	return snap.PlayBlocked
}

func updateDepsPanelVisibility(w *shellWidgets, snap clienthost.DependenciesSnapshot, depsChecking bool) {
	right := w.joinPanelRoot
	if depsPanelNeeded(snap, depsChecking) {
		right = w.depsPanel.Root
	}
	ui.SetPageSections(w.pageBox, container.NewGridWithColumns(2, w.hostPanelRoot, right))
}
