// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// logLevelString returns a human-readable label for a log level.
func logLevelString(level int) string {
	switch {
	case level <= -4:
		return "DEBUG"
	case level <= 0:
		return "INFO"
	case level <= 4:
		return "WARN"
	default:
		return "ERROR"
	}
}

// logLevelStyle returns the lipgloss style for a log level.
func logLevelStyle(level int, styles Styles) lipgloss.Style {
	switch {
	case level <= -4:
		return styles.Muted
	case level <= 0:
		return styles.Value
	case level <= 4:
		return styles.StatusWarn
	default:
		return styles.StateError
	}
}

// renderLogOverlay renders the system log viewer overlay.
func renderLogOverlay(entries []LogEntry, scrollOffset int, styles Styles, width, height int) string {
	var b strings.Builder

	b.WriteString(styles.Title.Render("System Log"))
	b.WriteString(fmt.Sprintf("  %s", styles.Muted.Render(fmt.Sprintf("(%d entries)", len(entries)))))
	b.WriteString("\n\n")

	if len(entries) == 0 {
		b.WriteString(styles.Muted.Render("  No log entries."))
		b.WriteString("\n\n")
		b.WriteString(styles.Muted.Render("  esc: close"))
		return b.String()
	}

	availableLines := height - 4
	if availableLines < 5 {
		availableLines = 5
	}

	// scrollOffset=0 means show newest entries at bottom (no scroll).
	// We show entries from oldest to newest, with scroll from the end.
	start := len(entries) - availableLines - scrollOffset
	if start < 0 {
		start = 0
	}
	end := start + availableLines
	if end > len(entries) {
		end = len(entries)
	}

	for i := start; i < end; i++ {
		e := entries[i]
		timeStr := e.When.Format("15:04:05")
		levelStr := logLevelString(e.Level)
		lvlStyle := logLevelStyle(e.Level, styles)

		line := fmt.Sprintf("  %s  %s  %s",
			styles.Muted.Render(timeStr),
			lvlStyle.Render(fmt.Sprintf("%-5s", levelStr)),
			e.Message,
		)

		if width > 0 && lipgloss.Width(line) > width-2 {
			// Truncate long lines
			runes := []rune(line)
			if len(runes) > width-5 {
				line = string(runes[:width-5]) + "..."
			}
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  j/k: scroll  esc: close"))

	return b.String()
}
