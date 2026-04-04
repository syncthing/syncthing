// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

func renderHelp(keys keyMap, styles Styles) string {
	groups := keys.FullHelp()
	headers := []string{"Tabs", "Lists", "Actions", "General"}

	// Split into left column (Tabs + Lists) and right column (Actions + General)
	leftGroups := groups[:2]
	rightGroups := groups[2:]

	left := renderHelpColumn(leftGroups, headers[:2], styles)
	right := renderHelpColumn(rightGroups, headers[2:], styles)

	return styles.Title.Render("Keyboard Shortcuts") + "\n\n" +
		lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", right) +
		"\n\n" + styles.Muted.Render("  Press ? to close")
}

func renderHelpColumn(groups [][]key.Binding, headers []string, styles Styles) string {
	var b strings.Builder
	for gi, group := range groups {
		if gi < len(headers) {
			b.WriteString(styles.Subtitle.Render("  "+headers[gi]) + "\n")
		}
		for _, binding := range group {
			if !binding.Enabled() {
				continue
			}
			h := binding.Help()
			b.WriteString(fmt.Sprintf("  %s  %s\n",
				styles.HelpKey.Render(fmt.Sprintf("%-12s", h.Key)),
				styles.HelpDesc.Render(h.Desc)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderShortHelp(keys keyMap, styles Styles) string {
	bindings := keys.ShortHelp()
	var parts []string
	for _, b := range bindings {
		if !b.Enabled() {
			continue
		}
		h := b.Help()
		parts = append(parts,
			styles.HelpKey.Render(h.Key)+styles.HelpDesc.Render(" "+h.Desc),
		)
	}
	return strings.Join(parts, styles.Muted.Render("  |  "))
}
