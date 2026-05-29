package fyneapp

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/michael4d45/bizshuffle/cmd/desktop/fyneapp/ui"
)

// shellState tracks UI-visible session state for panel styling and control gates.
type shellState struct {
	busy         bool
	installing   bool
	depsChecking bool
	hosting      bool

	statusText string
	statusSev  ui.StatusSeverity
}

func (s *shellState) setStatus(text string, sev ui.StatusSeverity) {
	s.statusText = text
	s.statusSev = sev
}

func (s *shellState) apply(w *shellWidgets, depsBlocked func() bool) {
	ui.SetStatus(w.status, s.statusText, s.statusSev)

	blocked := s.depsChecking
	if !blocked && depsBlocked != nil {
		blocked = depsBlocked()
	}
	if s.installing || blocked || s.busy {
		w.joinBtn.Disable()
	} else {
		w.joinBtn.Enable()
	}
	if s.busy {
		w.hostBtn.Disable()
	} else {
		w.hostBtn.Enable()
	}
	if s.hosting {
		w.stopHostBtn.Show()
	} else {
		w.stopHostBtn.Hide()
	}
	if s.installing || s.busy {
		w.refreshDiscoveryBtn.Disable()
	} else {
		w.refreshDiscoveryBtn.Enable()
	}
}

// shellWidgets holds shell controls and section panels.
type shellWidgets struct {
	root fyne.CanvasObject

	status *widget.Label

	hostEntry       *widget.Entry
	portEntry       *widget.Entry
	serverURLEntry  *widget.Entry
	playerNameEntry *widget.Entry
	hostBtn         *widget.Button
	stopHostBtn     *widget.Button
	joinBtn         *widget.Button
	versionLabel    *widget.Label
	updateBtn       *widget.Button
	checkUpdatesBtn *widget.Button
	openDataBtn     *widget.Button

	pageBox              *fyne.Container
	hostJoinRow          fyne.CanvasObject
	hostPanelRoot        fyne.CanvasObject
	joinPanelRoot        fyne.CanvasObject
	depsPanel            *ui.SectionPanel
	discoveryPanel       *ui.SectionPanel
	discoveryList        *widget.List
	discoveryEmpty       *widget.Label
	refreshDiscoveryBtn  *widget.Button
}
