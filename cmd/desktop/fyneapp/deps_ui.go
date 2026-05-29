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
		btn.Importance = widget.MediumImportance
		if installing {
			btn.Disable()
		}
		rows = append(rows, btn)
	}
	for _, item := range snap.Items {
		it := item
		action := widget.NewButton(it.ActionLabel, func() { onInstallOne(it) })
		action.Importance = widget.LowImportance
		if installing {
			action.Disable()
		}
		rows = append(rows, ui.NewInspectorRow(it.Label, it.Detail, action))
	}
	w.depsPanel.SetBody(container.NewVBox(rows...))

	if snap.PlayBlocked && len(snap.Items) > 0 {
		w.depsPanel.SetFooter(ui.NewMuted(clienthost.PlayBlockedMessage(snap)))
	} else {
		w.depsPanel.SetFooter(nil)
	}
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
	show := depsPanelNeeded(snap, depsChecking)
	sections := []fyne.CanvasObject{w.hostJoinRow}
	if show {
		sections = append(sections, w.depsPanel.Root)
	}
	sections = append(sections, w.discoveryPanel.Root)
	ui.SetPageSections(w.pageBox, sections...)
}

func setDiscoveryEmptyVisible(w *shellWidgets, empty bool) {
	if empty {
		w.discoveryEmpty.Show()
	} else {
		w.discoveryEmpty.Hide()
	}
}
