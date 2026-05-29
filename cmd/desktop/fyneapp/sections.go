package fyneapp

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/michael4d45/bizshuffle/cmd/desktop/fyneapp/ui"
)

// buildShell constructs the desktop layout and returns widget handles.
// discovered is the backing slice for the discovery list length and updates.
func buildShell(discovered *[]DiscoveredServer) *shellWidgets {
	w := &shellWidgets{
		status: ui.NewStatus("Host a session or join one as a player.", ui.StatusSeverityInfo),

		hostEntry:       widget.NewEntry(),
		portEntry:       widget.NewEntry(),
		serverURLEntry:  widget.NewEntry(),
		playerNameEntry: widget.NewEntry(),
		hostBtn:         widget.NewButton("Host (server + admin)", nil),
		stopHostBtn:     widget.NewButton("Stop host", nil),
		joinBtn:         widget.NewButton("Join", nil),
		versionLabel:    widget.NewLabel(""),
		updateBtn:       widget.NewButton("Download update", nil),
		checkUpdatesBtn: widget.NewButton("Check updates", nil),
		openDataBtn:     widget.NewButton("Open data folder", nil),
		discoveryEmpty:  widget.NewLabel("No servers found — refresh or enter a URL above."),
	}

	w.serverURLEntry.SetPlaceHolder("http://127.0.0.1:8080")
	w.playerNameEntry.SetPlaceHolder("Player name")
	w.stopHostBtn.Importance = widget.LowImportance
	w.stopHostBtn.Hide()
	w.joinBtn.Importance = widget.HighImportance
	w.hostBtn.Importance = widget.HighImportance
	w.updateBtn.Importance = widget.HighImportance
	w.updateBtn.Hide()
	w.discoveryEmpty.Importance = widget.LowImportance

	w.refreshDiscoveryBtn = widget.NewButton("Refresh", nil)
	w.refreshDiscoveryBtn.Importance = widget.LowImportance

	hostForm := widget.NewForm(
		widget.NewFormItem("Bind host", w.hostEntry),
		widget.NewFormItem("Port (0 = free)", w.portEntry),
	)
	hostPanel := ui.NewSectionPanel(
		"Host",
		"Embedded server and admin UI in your browser",
		w.stopHostBtn,
		hostForm,
		ui.NewActionBar(w.hostBtn),
	)
	w.hostPanelRoot = hostPanel.Root

	joinForm := widget.NewForm(
		widget.NewFormItem("Server URL", w.serverURLEntry),
		widget.NewFormItem("Player name", w.playerNameEntry),
	)
	joinPanel := ui.NewSectionPanel(
		"Join",
		"Connect as a player with BizHawk",
		nil,
		joinForm,
		ui.NewActionBar(w.joinBtn),
	)
	w.joinPanelRoot = joinPanel.Root
	w.hostJoinRow = container.NewGridWithColumns(2, w.hostPanelRoot, w.joinPanelRoot)

	w.depsPanel = ui.NewSectionPanel("Dependencies", "Required before joining", nil, nil, nil)

	w.discoveryList = widget.NewList(
		func() int { return len(*discovered) },
		func() fyne.CanvasObject { return ui.NewDiscoveryRow() },
		func(i int, o fyne.CanvasObject) {
			if i >= len(*discovered) {
				return
			}
			row, ok := o.(*ui.DiscoveryRow)
			if !ok {
				return
			}
			s := (*discovered)[i]
			ui.UpdateDiscoveryRow(row, s.Label, s.URL)
		},
	)

	listScroll := container.NewScroll(w.discoveryList)
	listScroll.SetMinSize(fyne.NewSize(0, ui.DiscoveryListMin))
	discoveryStack := container.NewStack(listScroll, w.discoveryEmpty)
	w.discoveryPanel = ui.NewSectionPanel(
		"Nearby servers",
		"LAN discovery and manual URL",
		w.refreshDiscoveryBtn,
		discoveryStack,
		nil,
	)

	w.pageBox = container.NewVBox()
	ui.SetPageSections(w.pageBox, w.hostJoinRow, w.discoveryPanel.Root)
	page := container.NewPadded(w.pageBox)

	header := ui.NewHeaderSurface("BizShuffle", nil)
	footerLeft := container.NewHBox(w.versionLabel, w.checkUpdatesBtn, w.updateBtn)
	footer := ui.NewFooterRow(footerLeft, w.openDataBtn)
	bottom := container.NewVBox(
		container.NewPadded(w.status),
		footer,
	)

	w.root = container.NewBorder(
		header,
		bottom,
		nil,
		nil,
		ui.NewScrollBody(page),
	)
	return w
}
