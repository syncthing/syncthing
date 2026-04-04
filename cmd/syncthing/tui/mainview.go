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

	"charm.land/lipgloss/v2"
)

// renderTabBar renders the tab bar at the top of the screen.
func renderTabBar(activeTab int, styles Styles, width int) string {
	tabs := []string{"Folders", "Devices", "Events"}
	var parts []string

	for i, name := range tabs {
		label := fmt.Sprintf(" %d:%s ", i+1, name)
		if i == activeTab {
			parts = append(parts, styles.SectionHeaderFocused.Render(label))
		} else {
			parts = append(parts, styles.SectionHeader.Render(label))
		}
	}

	bar := strings.Join(parts, styles.Muted.Render(" | "))

	// Pad the bar to full width
	barWidth := lipgloss.Width(bar)
	if barWidth < width {
		bar += strings.Repeat(" ", width-barWidth)
	}

	return bar
}

// renderFoldersTab renders the Folders tab content.
// It returns the rendered content, the line where the focused item starts,
// and the line where the focused item ends (including expanded content).
func renderFoldersTab(m *appModel) (string, int, int) {
	var b strings.Builder
	s := &m.state
	styles := m.styles
	focusLine := 0
	focusEndLine := 0

	lineCount := 0
	addContent := func(s string) {
		b.WriteString(s)
		lineCount += strings.Count(s, "\n")
	}

	markFocusEnd := func() {
		focusEndLine = lineCount
	}

	if len(s.Folders) == 0 && len(s.PendingFldrs) == 0 {
		focusLine = lineCount
		addContent(styles.Muted.Render("  No folders configured. Press 'a' to add one."))
		addContent("\n")
	} else {
		for i, f := range s.Folders {
			isFocused := m.folderCursor == i
			isExpanded := m.expandedFolders[i]
			if isFocused {
				focusLine = lineCount
			}
			addContent(renderFolderAccordion(&f, s, isFocused, isExpanded, styles))
			if isFocused {
				markFocusEnd()
			}
		}
	}

	// Pending folders (navigable -- cursor continues from configured folders)
	if len(s.PendingFldrs) > 0 {
		addContent("\n")
		addContent(styles.PendingBadge.Render("  Pending Folders"))
		addContent("\n")
		folderCount := len(s.Folders)
		for pi, p := range s.PendingFldrs {
			cursorIdx := folderCount + pi
			isFocused := m.folderCursor == cursorIdx
			if isFocused {
				focusLine = lineCount
			}
			name := p.Label
			if name == "" {
				name = p.FolderID
			}
			fromDev := p.DeviceName
			if fromDev == "" {
				fromDev = shortDeviceID(p.DeviceID)
			}
			line := fmt.Sprintf("  %s %s  from %s",
				styles.PendingBadge.Render("\u25b6 Pending"),
				name,
				styles.Muted.Render(fromDev),
			)
			if isFocused {
				addContent(styles.Selected.Render(line + "  [enter: accept, x: dismiss]"))
			} else {
				addContent(line)
			}
			addContent("\n")
			if isFocused {
				markFocusEnd()
			}
		}
		addContent(styles.Muted.Render("  enter: accept  x: dismiss"))
		addContent("\n")
	}

	// Folder action buttons
	addContent(renderFolderButtons(styles))
	addContent("\n")
	markFocusEnd()

	// System errors
	if len(s.SystemErrors) > 0 {
		addContent("\n")
		addContent(styles.StateError.Render(fmt.Sprintf("  Errors (%d)", len(s.SystemErrors))))
		addContent("\n")
		for _, e := range s.SystemErrors {
			addContent("  " + styles.StateError.Render(e.Message))
			addContent("\n")
		}
	}

	if s.RestartRequired {
		addContent("\n")
		addContent("  " + styles.StatusWarn.Render("Restart required to apply configuration changes"))
		addContent("\n")
	}

	return b.String(), focusLine, focusEndLine
}

// renderDevicesTab renders the Devices tab content.
func renderDevicesTab(m *appModel) (string, int, int) {
	var b strings.Builder
	s := &m.state
	styles := m.styles
	focusLine := 0
	focusEndLine := 0

	lineCount := 0
	addContent := func(s string) {
		b.WriteString(s)
		lineCount += strings.Count(s, "\n")
	}

	markFocusEnd := func() {
		focusEndLine = lineCount
	}

	// -- This Device section (always shown at top) --
	addContent(renderSectionHeader("This Device", styles, m.width))
	addContent("\n")
	addContent(renderThisDevice(s, styles))

	// -- Remote Devices section --
	addContent("\n")
	addContent(renderSectionHeader("Remote Devices", styles, m.width))
	addContent("\n")

	remoteDevices := remoteDeviceIndices(s)

	if len(remoteDevices) == 0 && len(s.PendingDevs) == 0 {
		focusLine = lineCount
		addContent(styles.Muted.Render("  No remote devices configured. Press 'a' to add one."))
		addContent("\n")
	}
	if len(remoteDevices) > 0 {
		for ci, devIdx := range remoteDevices {
			d := &s.Devices[devIdx]
			isFocused := m.deviceCursor == ci
			isExpanded := m.expandedDevices[ci]
			if isFocused {
				focusLine = lineCount
			}
			addContent(renderDeviceAccordion(d, s, isFocused, isExpanded, styles))
			if isFocused {
				markFocusEnd()
			}
		}
	}

	// Pending devices (navigable -- cursor continues from remote devices)
	if len(s.PendingDevs) > 0 {
		addContent("\n")
		addContent(styles.PendingBadge.Render("  Pending Devices"))
		addContent("\n")
		remoteCount := len(remoteDevices)
		for pi, p := range s.PendingDevs {
			cursorIdx := remoteCount + pi
			isFocused := m.deviceCursor == cursorIdx
			if isFocused {
				focusLine = lineCount
			}
			name := p.Name
			if name == "" {
				name = shortDeviceID(p.DeviceID)
			}
			line := fmt.Sprintf("  %s %s  %s",
				styles.PendingBadge.Render("\u25b6 Pending"),
				name,
				styles.Muted.Render(p.Address),
			)
			if isFocused {
				addContent(styles.Selected.Render(line + "  [enter: accept, x: dismiss]"))
			} else {
				addContent(line)
			}
			addContent("\n")
			if isFocused {
				markFocusEnd()
			}
		}
		addContent(styles.Muted.Render("  enter: accept  x: dismiss"))
		addContent("\n")
	}

	// Device action buttons
	addContent(renderDeviceButtons(styles))
	addContent("\n")
	markFocusEnd()

	return b.String(), focusLine, focusEndLine
}

// renderEventsTab renders the Events tab content (full-screen scrollable event log).
func renderEventsTab(s *AppState, elm *eventLogModel, styles Styles, width, height int) string {
	var b strings.Builder

	// Recent file changes section
	recentLinesUsed := 0
	if len(s.RecentChanges) > 0 {
		b.WriteString("  " + styles.Subtitle.Render("Recent File Changes"))
		b.WriteString("\n")
		recentLinesUsed++

		// Show most recent changes first, up to 10
		limit := len(s.RecentChanges)
		start := limit - 10
		if start < 0 {
			start = 0
		}
		for i := limit - 1; i >= start; i-- {
			rc := s.RecentChanges[i]
			timeStr := rc.Time.Format("15:04:05")
			folderName := rc.Folder
			if f := s.findFolder(rc.Folder); f != nil {
				folderName = f.DisplayName()
			}
			actionStyle := styles.StateIdle
			if rc.Action == "delete" {
				actionStyle = styles.StateError
			}
			line := fmt.Sprintf("  %s  %s  %-30s in %s",
				styles.Muted.Render(timeStr),
				actionStyle.Render(fmt.Sprintf("%-8s", rc.Action)),
				rc.Path,
				styles.Muted.Render(folderName),
			)
			if width > 0 && len(line) > width-2 {
				line = line[:width-5] + "..."
			}
			b.WriteString(line)
			b.WriteString("\n")
			recentLinesUsed++
		}
		b.WriteString("\n")
		recentLinesUsed++
	}

	// Event log header
	b.WriteString(fmt.Sprintf("  %s", styles.Muted.Render(fmt.Sprintf("Event Log (%d events)", len(s.EventLog)))))
	b.WriteString("\n\n")

	if len(s.EventLog) == 0 {
		b.WriteString(styles.Muted.Render("  No events yet. Events will appear as they occur."))
		return b.String()
	}

	availableLines := height - 4 - recentLinesUsed
	if availableLines < 5 {
		availableLines = 5
	}

	start := len(s.EventLog) - 1 - elm.scrollOffset
	if start < 0 {
		start = 0
	}
	end := start - availableLines
	if end < 0 {
		end = -1
	}

	for i := start; i > end; i-- {
		e := s.EventLog[i]
		timeStr := e.Time.Format("15:04:05")
		typeStyle := eventTypeStyle(e.Type, styles)
		typePadded := fmt.Sprintf("%-25s", e.Type)

		line := fmt.Sprintf("  %s  %s  %s",
			styles.Muted.Render(timeStr),
			typeStyle.Render(typePadded),
			e.Summary,
		)

		if width > 0 && len(line) > width-2 {
			line = line[:width-5] + "..."
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  j/k: scroll"))

	return b.String()
}

// renderSectionHeader renders a section header with border styling (non-focused, for subsections within tabs).
func renderSectionHeader(title string, styles Styles, width int) string {
	headerStyle := styles.SectionHeader

	lineWidth := width - len(title) - 5
	if lineWidth < 4 {
		lineWidth = 4
	}

	return headerStyle.Render(fmt.Sprintf(" %s %s", title, strings.Repeat("\u2500", lineWidth)))
}

// renderFolderAccordion renders a single folder as a collapsible accordion item.
func renderFolderAccordion(f *FolderState, s *AppState, focused, expanded bool, styles Styles) string {
	var b strings.Builder

	// Collapse/expand indicator
	indicator := "\u25b6" // collapsed: right-pointing triangle
	if expanded {
		indicator = "\u25bc" // expanded: down-pointing triangle
	}

	// Status badge
	stateStyle := folderStateStyle(*f, styles)
	state := f.State
	if f.Paused {
		state = "paused"
		stateStyle = styles.StatePaused
	}
	if state == "" {
		state = "unknown"
		stateStyle = styles.Muted
	}
	// Normalize "idle" to "Up to Date" for web GUI familiarity
	displayState := folderDisplayState(state)

	// Show sync percentage when folder is syncing and has outstanding bytes
	if (state == "syncing" || state == "sync-preparing") && f.Summary != nil && f.Summary.NeedBytes > 0 {
		displayState = fmt.Sprintf("Syncing (%s)", formatPercent(f.SyncPercent()))
	}

	name := f.DisplayName()

	line := fmt.Sprintf("  %s %s  %s",
		indicator,
		stateStyle.Render(fmt.Sprintf("%-13s", displayState)),
		name,
	)

	if focused {
		b.WriteString(styles.Selected.Render(line))
	} else {
		b.WriteString(line)
	}
	b.WriteString("\n")

	// Expanded detail
	if expanded {
		b.WriteString(renderFolderExpandedDetail(f, s, styles))
	}

	return b.String()
}

// renderFolderExpandedDetail renders the detail rows for an expanded folder.
func renderFolderExpandedDetail(f *FolderState, s *AppState, styles Styles) string {
	var b strings.Builder
	indent := "      "

	rows := [][2]string{
		{"Folder ID", f.ID},
		{"Folder Path", f.Path},
	}

	// Global/Local state
	if f.Summary != nil {
		globalStr := fmt.Sprintf("%s, %s, ~%s",
			pluralize(f.Summary.GlobalFiles, "file", "files"),
			pluralize(f.Summary.GlobalDirectories, "dir", "dirs"),
			formatBytes(f.Summary.GlobalBytes),
		)
		localStr := fmt.Sprintf("%s, %s, ~%s",
			pluralize(f.Summary.LocalFiles, "file", "files"),
			pluralize(f.Summary.LocalDirectories, "dir", "dirs"),
			formatBytes(f.Summary.LocalBytes),
		)
		rows = append(rows,
			[2]string{"Global State", globalStr},
			[2]string{"Local State", localStr},
		)

		if f.Summary.NeedBytes > 0 || f.Summary.NeedFiles > 0 {
			needStr := fmt.Sprintf("%s, %s (%s)",
				pluralize(f.Summary.NeedFiles, "file", "files"),
				formatBytes(f.Summary.NeedBytes),
				formatPercent(f.SyncPercent()),
			)
			rows = append(rows, [2]string{"Out of Sync", styles.StateSyncing.Render(needStr)})
		}
	}

	rows = append(rows, [2]string{"Folder Type", folderTypeString(f.Type)})

	// Rescan interval
	if f.RescanIntervalS > 0 {
		rescanStr := formatDuration(time.Duration(f.RescanIntervalS) * time.Second)
		if f.FSWatcherEnabled {
			rescanStr += "  Enabled"
		}
		rows = append(rows, [2]string{"Rescans", rescanStr})
	}

	// File pull order
	if f.PullOrder != "" {
		rows = append(rows, [2]string{"File Pull Order", pullOrderDisplay(f.PullOrder)})
	}

	if !f.LastScan.IsZero() {
		rows = append(rows, [2]string{"Last Scan", f.LastScan.Format("2006-01-02 15:04:05")})
	}
	if f.LastFile != "" {
		lastFileStr := f.LastFile
		if !f.LastFileAt.IsZero() {
			lastFileStr += fmt.Sprintf(" (%s)", f.LastFileAt.Format("2006-01-02 15:04:05"))
		}
		rows = append(rows, [2]string{"Last File", lastFileStr})
	}

	if f.Error != "" {
		rows = append(rows, [2]string{"Error", styles.StateError.Render(f.Error)})
	}

	// Shared with
	if len(f.DeviceIDs) > 0 {
		var devNames []string
		for _, devID := range f.DeviceIDs {
			if devID == s.MyID {
				continue
			}
			for _, d := range s.Devices {
				if d.ID == devID {
					devNames = append(devNames, d.DisplayName())
					break
				}
			}
		}
		if len(devNames) > 0 {
			rows = append(rows, [2]string{"Shared With", strings.Join(devNames, ", ")})
		}
	}

	for _, r := range rows {
		b.WriteString(fmt.Sprintf("%s%s  %s\n",
			indent,
			styles.Label.Render(fmt.Sprintf("%-14s", r[0])),
			r[1],
		))
	}

	// Folder errors (show all; viewport scrolling handles overflow)
	if len(f.Errors) > 0 {
		b.WriteString(fmt.Sprintf("%s%s\n", indent,
			styles.StateError.Render(fmt.Sprintf("Errors (%d)", len(f.Errors)))))
		for _, e := range f.Errors {
			b.WriteString(fmt.Sprintf("%s  %s: %s\n",
				indent,
				styles.Muted.Render(e.Path),
				styles.StateError.Render(e.Error),
			))
		}
	}

	// Per-folder action hints
	hint := indent + styles.Muted.Render("[E:edit] [p:pause] [s:scan] [x:remove] [S:share]")
	if f.Type == "sendonly" {
		hint += " " + styles.Muted.Render("[O:override]")
	}
	if f.Type == "receiveonly" {
		hint += " " + styles.Muted.Render("[V:revert]")
	}
	b.WriteString(hint)
	b.WriteString("\n")

	return b.String()
}

// renderThisDevice renders the "This Device" section content (always expanded).
func renderThisDevice(s *AppState, styles Styles) string {
	var b strings.Builder
	indent := "    "

	// Device name
	var thisDevName string
	for _, d := range s.Devices {
		if d.ID == s.MyID {
			thisDevName = d.DisplayName()
			break
		}
	}
	if thisDevName == "" {
		thisDevName = shortDeviceID(s.MyID)
	}
	b.WriteString(fmt.Sprintf("  %s\n", styles.Value.Render(thisDevName)))

	// Compute aggregate transfer totals and rates across all devices.
	var dlTotal, ulTotal int64
	var dlRate, ulRate float64
	for _, d := range s.Devices {
		if d.ID == s.MyID {
			dlTotal = d.InBytesTotal
			ulTotal = d.OutBytesTotal
		} else {
			dlRate += d.InBytesRate
			ulRate += d.OutBytesRate
		}
	}

	// Local state totals
	var totalFiles, totalDirs int
	var totalBytes int64
	for _, f := range s.Folders {
		if f.Summary != nil {
			totalFiles += f.Summary.LocalFiles
			totalDirs += f.Summary.LocalDirectories
			totalBytes += f.Summary.LocalBytes
		}
	}

	rows := [][2]string{
		{"Download Rate", fmt.Sprintf("%s (%s)", formatRate(dlRate), formatBytes(dlTotal))},
		{"Upload Rate", fmt.Sprintf("%s (%s)", formatRate(ulRate), formatBytes(ulTotal))},
	}
	if totalFiles > 0 || totalDirs > 0 {
		rows = append(rows, [2]string{"Local State", fmt.Sprintf("%s, %s, ~%s",
			pluralize(totalFiles, "file", "files"),
			pluralize(totalDirs, "dir", "dirs"),
			formatBytes(totalBytes),
		)})
	}
	if !s.StartTime.IsZero() {
		rows = append(rows, [2]string{"Uptime", formatDuration(time.Since(s.StartTime))})
	}
	rows = append(rows, [2]string{"Identification", shortDeviceID(s.MyID)})
	if s.Version != "" {
		rows = append(rows, [2]string{"Version", s.Version})
	}

	for _, r := range rows {
		b.WriteString(fmt.Sprintf("%s%s  %s\n",
			indent,
			styles.Label.Render(fmt.Sprintf("%-16s", r[0])),
			styles.Value.Render(r[1]),
		))
	}

	// Listener details
	if s.ListenersTotal > 0 {
		b.WriteString(fmt.Sprintf("%s%s  %s\n",
			indent,
			styles.Label.Render(fmt.Sprintf("%-16s", "Listeners")),
			styles.Value.Render(fmt.Sprintf("%d/%d", s.ListenersRunning, s.ListenersTotal)),
		))
		for _, ld := range s.ListenerDetails {
			status := styles.StateIdle.Render("OK")
			detail := ""
			if ld.Error != "" {
				status = styles.StateError.Render("Error")
				detail = " - " + styles.StateError.Render(ld.Error)
			} else if len(ld.LANAddresses) > 0 {
				detail = " (" + strings.Join(ld.LANAddresses, ", ") + ")"
			}
			b.WriteString(fmt.Sprintf("%s  %s - %s%s\n", indent, styles.Muted.Render(ld.Name), status, detail))
		}
	}

	// Discovery details
	if s.DiscoveryTotal > 0 {
		b.WriteString(fmt.Sprintf("%s%s  %s\n",
			indent,
			styles.Label.Render(fmt.Sprintf("%-16s", "Discovery")),
			styles.Value.Render(fmt.Sprintf("%d/%d", s.DiscoveryRunning, s.DiscoveryTotal)),
		))
		for _, dd := range s.DiscoveryDetails {
			status := styles.StateIdle.Render("OK")
			detail := ""
			if dd.Error != "" {
				status = styles.StateError.Render("Error")
				detail = " - " + styles.StateError.Render(dd.Error)
			}
			b.WriteString(fmt.Sprintf("%s  %s - %s%s\n", indent, styles.Muted.Render(dd.Name), status, detail))
		}
	}

	return b.String()
}

// renderDeviceAccordion renders a single remote device as a collapsible accordion item.
func renderDeviceAccordion(d *DeviceState, s *AppState, focused, expanded bool, styles Styles) string {
	var b strings.Builder

	indicator := "\u25b6"
	if expanded {
		indicator = "\u25bc"
	}

	var statusLipStyle lipgloss.Style
	switch {
	case d.Paused:
		statusLipStyle = styles.DevicePaused
	case d.Connected:
		statusLipStyle = styles.DeviceConnected
	default:
		statusLipStyle = styles.DeviceDisconnected
	}

	displayStatus := deviceDisplayState(d)

	// Show sync percentage when device is connected and not fully synced
	if d.Connected && !d.Paused && d.Completion > 0 && d.Completion < 100 {
		displayStatus = fmt.Sprintf("Syncing (%s)", formatPercent(d.Completion))
		statusLipStyle = styles.DeviceConnected
	}

	line := fmt.Sprintf("  %s %s  %s",
		indicator,
		statusLipStyle.Render(fmt.Sprintf("%-14s", displayStatus)),
		d.DisplayName(),
	)

	if focused {
		b.WriteString(styles.Selected.Render(line))
	} else {
		b.WriteString(line)
	}
	b.WriteString("\n")

	if expanded {
		b.WriteString(renderDeviceExpandedDetail(d, s, styles))
	}

	return b.String()
}

// renderDeviceExpandedDetail renders the detail rows for an expanded remote device.
func renderDeviceExpandedDetail(d *DeviceState, s *AppState, styles Styles) string {
	var b strings.Builder
	indent := "      "

	var rows [][2]string

	if d.Connected {
		rows = append(rows,
			[2]string{"Download Rate", fmt.Sprintf("%s (%s)", formatRate(d.InBytesRate), formatBytes(d.InBytesTotal))},
			[2]string{"Upload Rate", fmt.Sprintf("%s (%s)", formatRate(d.OutBytesRate), formatBytes(d.OutBytesTotal))},
		)
	}

	if d.Address != "" {
		rows = append(rows, [2]string{"Address", d.Address})
	}
	if d.ConnectionType != "" {
		rows = append(rows, [2]string{"Connection Type", connectionTypeDisplay(d.ConnectionType)})
	}
	if d.NumConnections > 0 {
		rows = append(rows, [2]string{"Connections", fmt.Sprintf("%d", d.NumConnections)})
	}
	if d.Compression != "" {
		rows = append(rows, [2]string{"Compression", compressionDisplay(d.Compression)})
	}
	rows = append(rows, [2]string{"Identification", shortDeviceID(d.ID)})
	if d.ClientVersion != "" {
		rows = append(rows, [2]string{"Version", d.ClientVersion})
	}
	if !d.LastSeen.IsZero() {
		rows = append(rows, [2]string{"Last Seen", d.LastSeen.Format("2006-01-02 15:04:05")})
	}

	// Shared folders
	if len(d.FolderIDs) > 0 {
		var folderNames []string
		for _, fid := range d.FolderIDs {
			for _, f := range s.Folders {
				if f.ID == fid {
					folderNames = append(folderNames, f.DisplayName())
					break
				}
			}
		}
		if len(folderNames) > 0 {
			rows = append(rows, [2]string{"Folders", strings.Join(folderNames, ", ")})
		}
	}

	for _, r := range rows {
		b.WriteString(fmt.Sprintf("%s%s  %s\n",
			indent,
			styles.Label.Render(fmt.Sprintf("%-16s", r[0])),
			r[1],
		))
	}

	// Per-device action hints
	b.WriteString(indent + styles.Muted.Render("[E:edit] [p:pause] [x:remove]"))
	b.WriteString("\n")

	return b.String()
}

// renderFolderButtons renders the folder section action buttons.
func renderFolderButtons(styles Styles) string {
	return styles.Muted.Render("  [a:Add Folder] [E:Edit] [s:Rescan] [p:Pause]")
}

// renderDeviceButtons renders the device section action buttons.
func renderDeviceButtons(styles Styles) string {
	return styles.Muted.Render("  [a:Add Remote Device] [E:Edit] [p:Pause]")
}

// remoteDeviceIndices returns indices into s.Devices for devices that are NOT
// this device. This is used to map deviceCursor to actual device indices.
func remoteDeviceIndices(s *AppState) []int {
	var indices []int
	for i, d := range s.Devices {
		if d.ID != s.MyID {
			indices = append(indices, i)
		}
	}
	return indices
}

// folderDisplayState converts internal state strings to web GUI display labels.
func folderDisplayState(state string) string {
	switch state {
	case "idle":
		return "Up to Date"
	case "syncing":
		return "Syncing"
	case "sync-preparing":
		return "Syncing"
	case "scanning":
		return "Scanning"
	case "scan-waiting":
		return "Scan Waiting"
	case "error":
		return "Error"
	case "paused":
		return "Paused"
	case "unknown":
		return "Unknown"
	default:
		return state
	}
}

// deviceDisplayState returns a display string for a device's connection status.
func deviceDisplayState(d *DeviceState) string {
	switch {
	case d.Paused:
		return "Paused"
	case d.Connected:
		return "Up to Date"
	default:
		return "Disconnected"
	}
}

// pullOrderDisplay converts internal pull order names to human-readable strings.
func pullOrderDisplay(order string) string {
	switch order {
	case "random":
		return "Random"
	case "alphabetic":
		return "Alphabetic"
	case "smallestFirst":
		return "Smallest First"
	case "largestFirst":
		return "Largest First"
	case "oldestFirst":
		return "Oldest First"
	case "newestFirst":
		return "Newest First"
	default:
		return order
	}
}

// compressionDisplay converts compression setting to human-readable string.
func compressionDisplay(comp string) string {
	switch comp {
	case "metadata":
		return "Metadata Only"
	case "always":
		return "All Data"
	case "never":
		return "Off"
	default:
		return comp
	}
}

// connectionTypeDisplay formats the connection type for display.
func connectionTypeDisplay(connType string) string {
	switch {
	case strings.HasPrefix(connType, "tcp"):
		return "TCP"
	case strings.HasPrefix(connType, "quic"):
		return "QUIC"
	case strings.HasPrefix(connType, "relay"):
		return "Relay"
	default:
		return connType
	}
}

// folderStateStyle returns the appropriate lipgloss style for a folder's state.
func folderStateStyle(f FolderState, styles Styles) lipgloss.Style {
	if f.Paused {
		return styles.StatePaused
	}
	switch f.State {
	case "idle":
		return styles.StateIdle
	case "syncing", "sync-preparing":
		return styles.StateSyncing
	case "scanning", "scan-waiting":
		return styles.StateScanning
	case "error":
		return styles.StateError
	default:
		return styles.Muted
	}
}
