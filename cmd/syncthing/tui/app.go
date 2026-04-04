// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

type connectionState int

const (
	connConnected connectionState = iota
	connDisconnected
)

// Tab indices
const (
	tabFolders = iota
	tabDevices
	tabEvents
	numTabs
)

type appModel struct {
	client APIClient
	state  AppState
	keys   keyMap
	styles Styles

	width          int
	height         int
	darkBG         bool
	connState      connectionState
	reconnectDelay time.Duration
	lastEventID    int
	statusMessage  string
	statusTime     time.Time

	// Lifecycle: prevent duplicate goroutines
	pollingActive bool // true when event poll goroutine is running
	tickingActive bool // true when refresh tick is scheduled

	// Tab-based view state
	activeTab       int
	folderCursor    int
	deviceCursor    int
	expandedFolders map[int]bool
	expandedDevices map[int]bool
	scrollOffset    int // vertical scroll offset for viewport

	// Event log state (now a tab, not overlay)
	eventLog eventLogModel

	// Modal state
	form     *formModel
	confirm  *confirmModel
	showHelp bool
	showID   bool // show device ID + QR code overlay

	// Log viewer overlay
	showLogs        bool
	logEntries      []LogEntry
	logScrollOffset int
}

func newApp(client APIClient) appModel {
	return appModel{
		client:          client,
		keys:            defaultKeyMap(),
		darkBG:          true, // default assumption; updated on BackgroundColorMsg
		styles:          newStyles(true),
		connState:       connConnected,
		expandedFolders: make(map[int]bool),
		expandedDevices: make(map[int]bool),
	}
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		fetchFullState(m.client),
	)
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.BackgroundColorMsg:
		m.darkBG = msg.IsDark()
		m.styles = newStyles(m.darkBG)
		return m, nil

	case fullStateMsg:
		if msg.err != nil {
			m.connState = connDisconnected
			m.reconnectDelay = reconnectMin
			return m, tryReconnect(m.client, m.reconnectDelay)
		}
		m.connState = connConnected
		if msg.state != nil {
			// Full initial load
			if msg.state.MyID != "" {
				m.state.MyID = msg.state.MyID
			}
			if msg.state.Version != "" {
				m.state.Version = msg.state.Version
			}
			m.state.StartTime = msg.state.StartTime
			m.state.Connected = true
			m.state.RestartRequired = msg.state.RestartRequired
			m.state.ListenersTotal = msg.state.ListenersTotal
			m.state.ListenersRunning = msg.state.ListenersRunning
			m.state.DiscoveryTotal = msg.state.DiscoveryTotal
			m.state.DiscoveryRunning = msg.state.DiscoveryRunning
			if msg.state.Folders != nil {
				m.state.Folders = msg.state.Folders
				m.expandedFolders = make(map[int]bool)
			}
			if msg.state.Devices != nil {
				m.state.Devices = msg.state.Devices
				m.expandedDevices = make(map[int]bool)
			}
			if msg.state.PendingDevs != nil {
				m.state.PendingDevs = msg.state.PendingDevs
			}
			if msg.state.PendingFldrs != nil {
				m.state.PendingFldrs = msg.state.PendingFldrs
			}
			m.state.DiscoveredDevices = msg.state.DiscoveredDevices
			m.state.SystemErrors = msg.state.SystemErrors
			m.state.LastUpdated = msg.state.LastUpdated

			// Clamp cursors
			m.clampFolderCursor()
			m.clampDeviceCursor()
		}
		// Only start poll/tick if not already running
		var cmds []tea.Cmd
		if !m.pollingActive {
			m.pollingActive = true
			cmds = append(cmds, pollEvents(m.client, m.lastEventID))
		}
		if !m.tickingActive {
			m.tickingActive = true
			cmds = append(cmds, tickConnections())
		}
		return m, tea.Batch(cmds...)

	case connectionsMsg:
		if msg.conns != nil {
			m.state.UpdateConnections(*msg.conns)
		}
		return m, nil

	case connectionsTickMsg:
		// Periodic connections fetch for bandwidth rates.
		// Schedule the next tick regardless of result.
		return m, tea.Batch(fetchConnections(m.client), tickConnections())

	case eventsMsg:
		var needPendingRefresh, needConfigRefresh bool
		for _, evt := range msg.events {
			entry := m.state.ProcessEvent(evt)
			if entry != nil && entry.Summary != "" {
				m.state.AddEventLog(*entry, maxEventLogSize)
			}
			if evt.Type == "PendingDevicesChanged" || evt.Type == "PendingFoldersChanged" {
				needPendingRefresh = true
			}
			if evt.Type == "ConfigSaved" {
				needConfigRefresh = true
			}
		}
		m.lastEventID = msg.lastID
		m.state.LastUpdated = time.Now()
		// Re-poll (the previous poll goroutine has finished)
		m.pollingActive = true
		if needConfigRefresh {
			// Config changed externally (e.g., introducer added devices,
			// web GUI edit). Do a full refresh to pick up new folders/devices.
			return m, tea.Batch(pollEvents(m.client, m.lastEventID), fetchFullState(m.client))
		}
		cmds := []tea.Cmd{pollEvents(m.client, m.lastEventID)}
		if needPendingRefresh {
			cmds = append(cmds, fetchPending(m.client))
		}
		return m, tea.Batch(cmds...)

	case pendingDevicesMsg:
		if msg.pending != nil {
			m.state.UpdatePendingDevices(msg.pending)
		}
		return m, nil

	case pendingMsg:
		if msg.devices != nil {
			m.state.UpdatePendingDevices(msg.devices)
		}
		if msg.folders != nil {
			m.state.UpdatePendingFoldersFromAPI(msg.folders)
		}
		m.clampFolderCursor()
		return m, nil

	case reconnectedMsg:
		m.connState = connConnected
		m.reconnectDelay = reconnectMin
		m.pollingActive = false
		m.tickingActive = false
		m.setStatus("Reconnected")
		return m, fetchFullState(m.client)

	case errMsg:
		switch msg.source {
		case "events":
			m.pollingActive = false
			m.tickingActive = false
			m.connState = connDisconnected
			m.state.Connected = false
			m.reconnectDelay = reconnectMin
			return m, tryReconnect(m.client, m.reconnectDelay)
		case "reconnect":
			m.reconnectDelay = min(m.reconnectDelay*2, reconnectMax)
			return m, tryReconnect(m.client, m.reconnectDelay)
		}
		return m, nil

	case folderErrorsMsg:
		m.state.UpdateFolderErrors(msg.folderID, msg.errors)
		return m, nil

	case folderConfigMsg:
		if msg.err != nil {
			m.setStatus("Error: " + msg.err.Error())
			return m, nil
		}
		f := m.state.findFolder(msg.cfg.ID)
		if f == nil {
			m.setStatus("Folder not found: " + msg.cfg.ID)
			return m, nil
		}
		form := newEditFolderForm(f, msg.cfg)
		m.form = &form
		return m, nil

	case deviceConfigMsg:
		if msg.err != nil {
			m.setStatus("Error: " + msg.err.Error())
			return m, nil
		}
		devID := msg.cfg.DeviceID.String()
		d := m.state.findDevice(devID)
		if d == nil {
			m.setStatus("Device not found: " + devID)
			return m, nil
		}
		form := newEditDeviceForm(d, msg.cfg)
		m.form = &form
		return m, nil

	case actionResultMsg:
		if msg.action == "" {
			// Silent action (e.g., fetching folder errors on expand)
			return m, nil
		}
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("Error: %s: %s", msg.action, msg.err))
		} else {
			m.setStatus(fmt.Sprintf("Done: %s", msg.action))
			// Full state refresh so config changes (add/remove/share) are
			// reflected immediately. The fullStateMsg handler won't spawn
			// duplicate goroutines because pollingActive/tickingActive guard.
			return m, fetchFullState(m.client)
		}
		return m, nil

	case logsMsg:
		if msg.err != nil {
			m.setStatus("Error fetching logs: " + msg.err.Error())
			return m, nil
		}
		m.logEntries = msg.entries
		m.logScrollOffset = 0
		m.showLogs = true
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *appModel) setStatus(msg string) {
	m.statusMessage = msg
	m.statusTime = time.Now()
}

func (m *appModel) clampFolderCursor() {
	totalItems := len(m.state.Folders) + len(m.state.PendingFldrs)
	if m.folderCursor >= totalItems {
		m.folderCursor = totalItems - 1
	}
	if m.folderCursor < 0 {
		m.folderCursor = 0
	}
}

func (m *appModel) clampDeviceCursor() {
	remoteCount := len(remoteDeviceIndices(&m.state)) + len(m.state.PendingDevs)
	if m.deviceCursor >= remoteCount {
		m.deviceCursor = remoteCount - 1
	}
	if m.deviceCursor < 0 {
		m.deviceCursor = 0
	}
}

func (m appModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C always quits, regardless of modal state
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Confirmation dialog takes priority
	if m.confirm != nil {
		return m.handleConfirmKey(msg)
	}

	// Form takes priority
	if m.form != nil {
		return m.handleFormKey(msg)
	}

	// Help overlay
	if m.showHelp {
		if key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Back) || key.Matches(msg, m.keys.Quit) {
			m.showHelp = false
		}
		return m, nil
	}

	// Show ID overlay
	if m.showID {
		if key.Matches(msg, m.keys.ShowID) || key.Matches(msg, m.keys.Back) || key.Matches(msg, m.keys.Quit) {
			m.showID = false
		}
		return m, nil
	}

	// Log viewer overlay
	if m.showLogs {
		switch {
		case key.Matches(msg, m.keys.Back) || key.Matches(msg, m.keys.Quit) || key.Matches(msg, m.keys.Logs):
			m.showLogs = false
		case key.Matches(msg, m.keys.Up):
			if m.logScrollOffset > 0 {
				m.logScrollOffset--
			}
		case key.Matches(msg, m.keys.Down):
			maxScroll := len(m.logEntries) - 1
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.logScrollOffset < maxScroll {
				m.logScrollOffset++
			}
		case key.Matches(msg, m.keys.PageUp):
			pageSize := m.height - 5
			if pageSize < 1 {
				pageSize = 1
			}
			m.logScrollOffset -= pageSize
			if m.logScrollOffset < 0 {
				m.logScrollOffset = 0
			}
		case key.Matches(msg, m.keys.PageDown):
			pageSize := m.height - 5
			if pageSize < 1 {
				pageSize = 1
			}
			maxScroll := len(m.logEntries) - 1
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.logScrollOffset += pageSize
			if m.logScrollOffset > maxScroll {
				m.logScrollOffset = maxScroll
			}
		}
		return m, nil
	}

	// Global keys
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.showHelp = true
		return m, nil
	case key.Matches(msg, m.keys.RestartDaemon):
		m.confirm = &confirmModel{
			kind:    confirmRestart,
			message: "Restart Syncthing daemon?",
		}
		return m, nil
	case key.Matches(msg, m.keys.ShutDown):
		m.confirm = &confirmModel{
			kind:    confirmShutdown,
			message: "Shut down Syncthing daemon?",
		}
		return m, nil
	case key.Matches(msg, m.keys.ClearErrors):
		if len(m.state.SystemErrors) > 0 {
			m.state.SystemErrors = nil
			return m, doAction("clear errors", func() error { return m.client.ErrorsClear() })
		}
		return m, nil
	case key.Matches(msg, m.keys.ShowID):
		m.showID = true
		return m, nil
	case key.Matches(msg, m.keys.Logs):
		return m, fetchLogs(m.client)

	// Tab switching with number keys
	case key.Matches(msg, m.keys.TabFolders):
		m.activeTab = tabFolders
		m.scrollOffset = 0
		return m, nil
	case key.Matches(msg, m.keys.TabDevices):
		m.activeTab = tabDevices
		m.scrollOffset = 0
		return m, nil
	case key.Matches(msg, m.keys.TabEvents):
		m.activeTab = tabEvents
		m.scrollOffset = 0
		return m, nil

	// Tab cycling with Tab / Shift-Tab
	case key.Matches(msg, m.keys.NextTab):
		m.activeTab = (m.activeTab + 1) % numTabs
		m.scrollOffset = 0
		return m, nil
	case key.Matches(msg, m.keys.PrevTab):
		m.activeTab = (m.activeTab - 1 + numTabs) % numTabs
		m.scrollOffset = 0
		return m, nil
	}

	// Page up/down for manual scrolling
	switch {
	case key.Matches(msg, m.keys.PageUp):
		pageSize := m.height - 5
		if pageSize < 1 {
			pageSize = 1
		}
		m.scrollOffset -= pageSize
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
		return m, nil
	case key.Matches(msg, m.keys.PageDown):
		pageSize := m.height - 5
		if pageSize < 1 {
			pageSize = 1
		}
		m.scrollOffset += pageSize
		return m, nil
	}

	// Tab-specific keys
	switch m.activeTab {
	case tabFolders:
		return m.handleFolderKey(msg)
	case tabDevices:
		return m.handleDeviceKey(msg)
	case tabEvents:
		return m.handleEventTabKey(msg)
	}

	return m, nil
}

func (m appModel) handleFolderKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	folderCount := len(m.state.Folders)
	pendingCount := len(m.state.PendingFldrs)
	totalItems := folderCount + pendingCount

	// Is the cursor on a pending folder?
	onPending := m.folderCursor >= folderCount && m.folderCursor < totalItems
	pendingIdx := m.folderCursor - folderCount

	switch {
	case key.Matches(msg, m.keys.Up):
		if totalItems > 0 {
			m.folderCursor = (m.folderCursor - 1 + totalItems) % totalItems
		}
	case key.Matches(msg, m.keys.Down):
		if totalItems > 0 {
			m.folderCursor = (m.folderCursor + 1) % totalItems
		}
	case key.Matches(msg, m.keys.Enter):
		if onPending {
			// Accept pending folder: open a pre-filled Accept Folder form
			p := m.state.PendingFldrs[pendingIdx]
			form := newAcceptFolderForm(p.FolderID, p.Label, p.DeviceID)
			m.form = &form
		} else if m.folderCursor < folderCount {
			// Toggle expand/collapse
			m.expandedFolders[m.folderCursor] = !m.expandedFolders[m.folderCursor]
			// Fetch folder errors if expanding
			if m.expandedFolders[m.folderCursor] {
				f := &m.state.Folders[m.folderCursor]
				return m, fetchFolderErrors(m.client, f.ID)
			}
		}
	case key.Matches(msg, m.keys.Add):
		form := newAddFolderForm()
		m.form = &form
	case key.Matches(msg, m.keys.Edit):
		if !onPending && m.folderCursor < folderCount {
			f := &m.state.Folders[m.folderCursor]
			return m, fetchFolderConfig(m.client, f.ID)
		}
	case key.Matches(msg, m.keys.Scan):
		if !onPending && m.folderCursor < folderCount {
			f := &m.state.Folders[m.folderCursor]
			return m, doAction("scan "+f.ID, func() error { return m.client.Scan(f.ID) })
		}
	case key.Matches(msg, m.keys.Pause):
		if !onPending && m.folderCursor < folderCount {
			f := &m.state.Folders[m.folderCursor]
			return m.toggleFolderPause(f)
		}
	case key.Matches(msg, m.keys.Remove):
		if onPending {
			// Dismiss pending folder
			p := m.state.PendingFldrs[pendingIdx]
			return m, doAction("dismiss pending folder", func() error {
				return m.client.PendingFolderDismiss(p.DeviceID, p.FolderID)
			})
		} else if m.folderCursor < folderCount {
			f := &m.state.Folders[m.folderCursor]
			c := newConfirm(confirmRemoveFolder, fmt.Sprintf("Remove folder %q?", f.DisplayName()), f.ID)
			m.confirm = &c
		}
	case key.Matches(msg, m.keys.Share):
		if !onPending && m.folderCursor < folderCount {
			f := &m.state.Folders[m.folderCursor]
			availableDevices := m.availableDevicesForFolder(f)
			form := newShareFolderForm(f.ID, availableDevices)
			m.form = &form
			return m, nil
		}
	case key.Matches(msg, m.keys.Override):
		if !onPending && m.folderCursor < folderCount {
			f := &m.state.Folders[m.folderCursor]
			if f.Type == "sendonly" {
				c := newConfirm(confirmOverride, fmt.Sprintf("Override changes in %q?", f.DisplayName()), f.ID)
				m.confirm = &c
			}
		}
	case key.Matches(msg, m.keys.Revert):
		if !onPending && m.folderCursor < folderCount {
			f := &m.state.Folders[m.folderCursor]
			if f.Type == "receiveonly" {
				c := newConfirm(confirmRevert, fmt.Sprintf("Revert changes in %q?", f.DisplayName()), f.ID)
				m.confirm = &c
			}
		}
	}
	return m, nil
}

func (m appModel) handleDeviceKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	indices := remoteDeviceIndices(&m.state)
	remoteCount := len(indices)
	pendingCount := len(m.state.PendingDevs)
	totalItems := remoteCount + pendingCount

	// Is the cursor on a pending device?
	onPending := m.deviceCursor >= remoteCount && m.deviceCursor < totalItems
	pendingIdx := m.deviceCursor - remoteCount

	switch {
	case key.Matches(msg, m.keys.Up):
		if totalItems > 0 {
			m.deviceCursor = (m.deviceCursor - 1 + totalItems) % totalItems
		}
	case key.Matches(msg, m.keys.Down):
		if totalItems > 0 {
			m.deviceCursor = (m.deviceCursor + 1) % totalItems
		}
	case key.Matches(msg, m.keys.Enter):
		if onPending {
			// Accept pending device: open a pre-filled Add Device form
			p := m.state.PendingDevs[pendingIdx]
			form := newAddDeviceForm([]DiscoveredDevice{{
				DeviceID:  p.DeviceID,
				Name:      p.Name,
				Addresses: []string{p.Address},
			}}, m.state.Folders)
			// Auto-select the discovered device
			form.discoveredDevices[0] = DiscoveredDevice{
				DeviceID: p.DeviceID, Name: p.Name, Addresses: []string{p.Address},
			}
			form.inputs[0].SetValue(p.DeviceID)
			if p.Name != "" {
				form.inputs[1].SetValue(p.Name)
			}
			if p.Address != "" {
				form.inputs[2].SetValue(p.Address)
			}
			form.discoveryFocused = false
			form.focus = 1 // focus on Name so user can confirm
			form.updateFocus()
			m.form = &form
		} else if m.deviceCursor < remoteCount {
			m.expandedDevices[m.deviceCursor] = !m.expandedDevices[m.deviceCursor]
		}
	case key.Matches(msg, m.keys.Add):
		// Merge discovered devices and pending devices into the nearby list
		nearby := make([]DiscoveredDevice, 0, len(m.state.DiscoveredDevices)+pendingCount)
		nearby = append(nearby, m.state.DiscoveredDevices...)
		for _, p := range m.state.PendingDevs {
			nearby = append(nearby, DiscoveredDevice{
				DeviceID:  p.DeviceID,
				Name:      p.Name,
				Addresses: []string{p.Address},
			})
		}
		form := newAddDeviceForm(nearby, m.state.Folders)
		m.form = &form
	case key.Matches(msg, m.keys.Edit):
		if !onPending && m.deviceCursor < remoteCount {
			d := &m.state.Devices[indices[m.deviceCursor]]
			return m, fetchDeviceConfig(m.client, d.ID)
		}
	case key.Matches(msg, m.keys.Pause):
		if !onPending && m.deviceCursor < remoteCount {
			d := &m.state.Devices[indices[m.deviceCursor]]
			return m.toggleDevicePause(d)
		}
	case key.Matches(msg, m.keys.Remove):
		if onPending {
			// Dismiss pending device
			p := m.state.PendingDevs[pendingIdx]
			return m, doAction("dismiss pending", func() error {
				return m.client.PendingDeviceDismiss(p.DeviceID)
			})
		} else if m.deviceCursor < remoteCount {
			d := &m.state.Devices[indices[m.deviceCursor]]
			c := newConfirm(confirmRemoveDevice, fmt.Sprintf("Remove device %q?", d.DisplayName()), d.ID)
			m.confirm = &c
		}
	}
	return m, nil
}

func (m appModel) handleEventTabKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Clamp scroll offset if the event log shrank (e.g. daemon restart).
	if m.eventLog.scrollOffset >= len(m.state.EventLog) {
		m.eventLog.scrollOffset = max(0, len(m.state.EventLog)-1)
	}

	switch {
	case key.Matches(msg, m.keys.Up):
		if m.eventLog.scrollOffset < len(m.state.EventLog)-1 {
			m.eventLog.scrollOffset++
		}
	case key.Matches(msg, m.keys.Down):
		if m.eventLog.scrollOffset > 0 {
			m.eventLog.scrollOffset--
		}
	}
	return m, nil
}

func (m appModel) handleFormKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.form = nil
		return m, nil
	case key.Matches(msg, m.keys.Enter):
		// When the discovery list is focused, Enter selects a device
		// rather than submitting the form.
		if m.form.discoveryFocused && len(m.form.discoveredDevices) > 0 {
			cmd := m.form.Update(msg)
			return m, cmd
		}
		return m.submitForm()
	}

	cmd := m.form.Update(msg)
	return m, cmd
}

func (m appModel) handleConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.ConfirmYes):
		confirm := m.confirm
		m.confirm = nil
		return m.executeConfirm(confirm)
	case key.Matches(msg, m.keys.ConfirmNo):
		m.confirm = nil
	}
	return m, nil
}

func (m appModel) submitForm() (tea.Model, tea.Cmd) {
	vals := m.form.values()
	kind := m.form.kind
	formFolderID := m.form.folderID
	formDeviceID := m.form.deviceID
	shareFolderIDs := m.form.selectedFolderIDs()
	m.form = nil

	switch kind {
	case formAcceptFolder:
		folderID := vals["Folder ID"]
		if folderID == "" {
			return m, nil
		}
		cfg := config.FolderConfiguration{
			ID:    folderID,
			Label: vals["Label"],
			Path:  vals["Path"],
		}
		if t := vals["Type"]; t != "" {
			cfg.Type = config.FolderType(folderTypeIndex(t))
		}
		// Include the offering device
		if formDeviceID != "" {
			devID, err := protocol.DeviceIDFromString(formDeviceID)
			if err == nil {
				cfg.Devices = []config.FolderDeviceConfiguration{{DeviceID: devID}}
			}
		}
		deviceID := formDeviceID
		return m, doAction("accept folder", func() error {
			if err := m.client.FolderAdd(cfg); err != nil {
				return err
			}
			// Dismiss the pending folder after adding
			if deviceID != "" {
				_ = m.client.PendingFolderDismiss(deviceID, folderID)
			}
			return nil
		})

	case formAddFolder:
		folderID := vals["Folder ID"]
		if folderID == "" {
			return m, nil
		}
		cfg := config.FolderConfiguration{
			ID:    folderID,
			Label: vals["Label"],
			Path:  vals["Path"],
		}
		if t := vals["Type"]; t != "" {
			cfg.Type = config.FolderType(folderTypeIndex(t))
		}
		return m, doAction("add folder", func() error { return m.client.FolderAdd(cfg) })

	case formAddDevice:
		devIDStr := vals["Device ID"]
		if devIDStr == "" {
			return m, nil
		}
		devID, err := protocol.DeviceIDFromString(devIDStr)
		if err != nil {
			m.setStatus("Invalid device ID: " + err.Error())
			return m, nil
		}
		addrs := []string{"dynamic"}
		if a := vals["Addresses"]; a != "" {
			addrs = strings.Split(a, ",")
			for i := range addrs {
				addrs[i] = strings.TrimSpace(addrs[i])
			}
		}
		cfg := config.DeviceConfiguration{
			DeviceID:  devID,
			Name:      vals["Name"],
			Addresses: addrs,
		}
		return m, doAction("add device", func() error {
			if err := m.client.DeviceAdd(cfg); err != nil {
				return err
			}
			// Share selected folders with the new device
			for _, folderID := range shareFolderIDs {
				folderCfg, err := m.client.ConfigFolderGet(folderID)
				if err != nil {
					continue
				}
				folderCfg.Devices = append(folderCfg.Devices, config.FolderDeviceConfiguration{
					DeviceID: devID,
				})
				if err := m.client.FolderUpdate(folderCfg); err != nil {
					continue
				}
			}
			return nil
		})

	case formShareFolder:
		devIDStr := vals["Device ID"]
		if devIDStr == "" {
			return m, nil
		}
		devID, err := protocol.DeviceIDFromString(devIDStr)
		if err != nil {
			m.setStatus("Invalid device ID: " + err.Error())
			return m, nil
		}
		folderID := formFolderID
		return m, doAction("share folder", func() error {
			cfg, err := m.client.ConfigGet()
			if err != nil {
				return err
			}
			for _, f := range cfg.Folders {
				if f.ID == folderID {
					f.Devices = append(f.Devices, config.FolderDeviceConfiguration{
						DeviceID: devID,
					})
					return m.client.FolderUpdate(f)
				}
			}
			return fmt.Errorf("folder %s not found", folderID)
		})

	case formEditFolder:
		folderID := formFolderID
		return m, doAction("edit folder", func() error {
			cfg, err := m.client.ConfigFolderGet(folderID)
			if err != nil {
				return err
			}
			cfg.Label = vals["Label"]
			if rescan := vals["Rescan Interval (seconds)"]; rescan != "" {
				var n int
				if _, err := fmt.Sscanf(rescan, "%d", &n); err == nil {
					cfg.RescanIntervalS = n
				}
			}
			if t := vals["Folder Type"]; t != "" {
				cfg.Type = config.FolderType(folderTypeIndex(t))
			}
			if po := vals["File Pull Order"]; po != "" {
				var order config.PullOrder
				_ = order.UnmarshalText([]byte(po))
				cfg.Order = order
			}
			if w := vals["FS Watcher"]; w != "" {
				cfg.FSWatcherEnabled = w == "true"
			}
			if p := vals["Paused"]; p != "" {
				cfg.Paused = p == "true"
			}
			return m.client.FolderUpdate(cfg)
		})

	case formEditDevice:
		deviceID := formDeviceID
		return m, doAction("edit device", func() error {
			devID, err := protocol.DeviceIDFromString(deviceID)
			if err != nil {
				return err
			}
			cfg, err := m.client.ConfigDeviceGet(devID.String())
			if err != nil {
				return err
			}
			cfg.Name = vals["Name"]
			if a := vals["Addresses"]; a != "" {
				addrs := strings.Split(a, ",")
				for i := range addrs {
					addrs[i] = strings.TrimSpace(addrs[i])
				}
				cfg.Addresses = addrs
			}
			if c := vals["Compression"]; c != "" {
				_ = cfg.Compression.UnmarshalText([]byte(c))
			}
			if v := vals["Introducer"]; v != "" {
				cfg.Introducer = v == "true"
			}
			if v := vals["Auto Accept Folders"]; v != "" {
				cfg.AutoAcceptFolders = v == "true"
			}
			if p := vals["Paused"]; p != "" {
				cfg.Paused = p == "true"
			}
			return m.client.DeviceUpdate(cfg)
		})
	}

	return m, nil
}

func (m appModel) executeConfirm(c *confirmModel) (tea.Model, tea.Cmd) {
	switch c.kind {
	case confirmRemoveFolder:
		return m, doAction("remove folder", func() error { return m.client.FolderRemove(c.id) })
	case confirmRemoveDevice:
		return m, doAction("remove device", func() error { return m.client.DeviceRemove(c.id) })
	case confirmRestart:
		return m, doAction("restart", func() error { return m.client.Restart() })
	case confirmShutdown:
		return m, tea.Batch(
			doAction("shut down", func() error { return m.client.Shutdown() }),
			func() tea.Msg { return tea.QuitMsg{} },
		)
	case confirmOverride:
		return m, doAction("override "+c.id, func() error { return m.client.Override(c.id) })
	case confirmRevert:
		return m, doAction("revert "+c.id, func() error { return m.client.Revert(c.id) })
	}
	return m, nil
}

func (m appModel) toggleFolderPause(f *FolderState) (tea.Model, tea.Cmd) {
	folderID := f.ID
	wasPaused := f.Paused
	return m, doAction("toggle pause", func() error {
		cfg, err := m.client.ConfigGet()
		if err != nil {
			return err
		}
		for i, fc := range cfg.Folders {
			if fc.ID == folderID {
				cfg.Folders[i].Paused = !wasPaused
				return m.client.FolderUpdate(cfg.Folders[i])
			}
		}
		return fmt.Errorf("folder %s not found", folderID)
	})
}

func (m appModel) toggleDevicePause(d *DeviceState) (tea.Model, tea.Cmd) {
	if d.Paused {
		return m, doAction("resume device", func() error { return m.client.Resume(d.ID) })
	}
	return m, doAction("pause device", func() error { return m.client.Pause(d.ID) })
}

// availableDevicesForFolder returns devices from the config that don't
// already share this folder (and are not the local device).
func (m *appModel) availableDevicesForFolder(f *FolderState) []DeviceState {
	sharing := make(map[string]bool, len(f.DeviceIDs))
	for _, id := range f.DeviceIDs {
		sharing[id] = true
	}
	var available []DeviceState
	for _, d := range m.state.Devices {
		if d.ID == m.state.MyID {
			continue
		}
		if sharing[d.ID] {
			continue
		}
		available = append(available, d)
	}
	return available
}

func folderTypeIndex(t string) int {
	for i, ft := range folderTypeOptions {
		if ft == t {
			return i
		}
	}
	return 0
}

func (m appModel) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	var content strings.Builder

	// Connection banner
	if m.connState != connConnected {
		content.WriteString(m.styles.ErrorBanner.Render("  Connection lost. Reconnecting..."))
		content.WriteString("\n")
	}

	// Main content area height (subtract tab bar + status bar + margins)
	contentHeight := m.height - 4 // tab bar + status bar + margins
	if contentHeight < 1 {
		contentHeight = 1
	}

	switch {
	case m.showLogs:
		content.WriteString(renderTabBar(m.activeTab, m.styles, m.width))
		content.WriteString("\n")
		content.WriteString(renderLogOverlay(m.logEntries, m.logScrollOffset, m.styles, m.width, contentHeight))
	case m.showHelp:
		content.WriteString(renderTabBar(m.activeTab, m.styles, m.width))
		content.WriteString("\n")
		content.WriteString(renderHelp(m.keys, m.styles))
	case m.showID:
		content.WriteString(renderTabBar(m.activeTab, m.styles, m.width))
		content.WriteString("\n")
		content.WriteString(renderIDOverlay(&m.state, m.styles, m.darkBG))
	case m.form != nil:
		content.WriteString(renderTabBar(m.activeTab, m.styles, m.width))
		content.WriteString("\n")
		content.WriteString(m.form.View(m.styles))
	case m.confirm != nil:
		content.WriteString(renderTabBar(m.activeTab, m.styles, m.width))
		content.WriteString("\n")
		content.WriteString(m.confirm.View(m.styles))
	default:
		// Tab bar
		content.WriteString(renderTabBar(m.activeTab, m.styles, m.width))
		content.WriteString("\n")

		var tabContent string
		var focusLine, focusEndLine int

		switch m.activeTab {
		case tabFolders:
			tabContent, focusLine, focusEndLine = renderFoldersTab(&m)
		case tabDevices:
			tabContent, focusLine, focusEndLine = renderDevicesTab(&m)
		case tabEvents:
			tabContent = renderEventsTab(&m.state, &m.eventLog, m.styles, m.width, contentHeight)
			focusLine = 0
			focusEndLine = 0
		}

		// Viewport scrolling: show content that fits in the available height
		lines := strings.Split(tabContent, "\n")
		totalLines := len(lines)

		// Auto-scroll for folders/devices tabs
		if m.activeTab != tabEvents {
			blockHeight := focusEndLine - focusLine
			if blockHeight <= contentHeight {
				if focusEndLine >= m.scrollOffset+contentHeight {
					m.scrollOffset = focusEndLine - contentHeight + 1
				}
			} else {
				if focusLine < m.scrollOffset || focusLine >= m.scrollOffset+contentHeight {
					m.scrollOffset = focusLine
				}
			}
			if focusLine < m.scrollOffset {
				m.scrollOffset = focusLine
			}
		}

		// Clamp scroll offset
		maxScroll := totalLines - contentHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.scrollOffset > maxScroll {
			m.scrollOffset = maxScroll
		}
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}

		endLine := m.scrollOffset + contentHeight
		if endLine > totalLines {
			endLine = totalLines
		}

		visible := lines[m.scrollOffset:endLine]
		content.WriteString(strings.Join(visible, "\n"))
	}

	// Status bar
	content.WriteString("\n")
	content.WriteString(m.renderStatusBar())

	v := tea.NewView(content.String())
	v.AltScreen = true
	v.WindowTitle = "Syncthing TUI"
	return v
}

func (m appModel) renderStatusBar() string {
	var left string
	switch m.connState {
	case connConnected:
		left = m.styles.StatusGood.Render("\u25cf") + " "
	default:
		left = m.styles.StatusBad.Render("\u2717") + " "
	}

	uptime := ""
	if !m.state.StartTime.IsZero() {
		uptime = " | Up " + formatDuration(time.Since(m.state.StartTime))
	}

	// Aggregate bandwidth rates across all devices
	var dlRate, ulRate float64
	for _, d := range m.state.Devices {
		dlRate += d.InBytesRate
		ulRate += d.OutBytesRate
	}
	bw := ""
	if dlRate > 0 || ulRate > 0 {
		bw = fmt.Sprintf(" | \u2193%s \u2191%s", formatRate(dlRate), formatRate(ulRate))
	}

	left += m.styles.StatusBar.Render(fmt.Sprintf("Syncthing %s | %s%s%s",
		m.state.Version,
		shortDeviceID(m.state.MyID),
		uptime,
		bw,
	))

	right := renderShortHelp(m.keys, m.styles)

	// Status message (clears after 5 seconds)
	if m.statusMessage != "" && time.Since(m.statusTime) < 5*time.Second {
		left += "  " + m.styles.StatusWarn.Render(m.statusMessage)
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
