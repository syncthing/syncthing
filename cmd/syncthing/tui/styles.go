// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"charm.land/lipgloss/v2"
)

// Styles holds all lipgloss styles used by the TUI, adaptive to dark/light
// terminal backgrounds. No gray text — hierarchy comes from bold and color,
// not reduced contrast.
type Styles struct {
	StatusBar  lipgloss.Style
	StatusGood lipgloss.Style
	StatusBad  lipgloss.Style
	StatusWarn lipgloss.Style

	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Label    lipgloss.Style
	Value    lipgloss.Style
	Muted    lipgloss.Style // same brightness, just not bold

	StateIdle     lipgloss.Style
	StateSyncing  lipgloss.Style
	StateError    lipgloss.Style
	StatePaused   lipgloss.Style
	StateScanning lipgloss.Style

	DeviceConnected    lipgloss.Style
	DeviceDisconnected lipgloss.Style
	DevicePaused       lipgloss.Style

	PendingBadge lipgloss.Style

	ErrorBanner lipgloss.Style
	HelpKey     lipgloss.Style
	HelpDesc    lipgloss.Style
	Selected    lipgloss.Style
	FormLabel   lipgloss.Style

	SectionHeader        lipgloss.Style
	SectionHeaderFocused lipgloss.Style
}

func newStyles(darkBG bool) Styles {
	ld := lipgloss.LightDark(darkBG)

	// Base foreground: full contrast against the background
	fg := ld(lipgloss.Color("#000"), lipgloss.Color("#FFF"))

	return Styles{
		StatusBar:  lipgloss.NewStyle().Foreground(fg),
		StatusGood: lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#080"), lipgloss.Color("#0E0"))),
		StatusBad:  lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#C00"), lipgloss.Color("#F77"))),
		StatusWarn: lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#A70"), lipgloss.Color("#FD0"))),

		Title:    lipgloss.NewStyle().Bold(true).Foreground(fg),
		Subtitle: lipgloss.NewStyle().Bold(true).Foreground(fg),
		Label:    lipgloss.NewStyle().Foreground(fg),
		Value:    lipgloss.NewStyle().Foreground(fg),
		Muted:    lipgloss.NewStyle().Foreground(fg),

		StateIdle:     lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#080"), lipgloss.Color("#0E0"))),
		StateSyncing:  lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#06B"), lipgloss.Color("#5CF"))),
		StateError:    lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#C00"), lipgloss.Color("#F77"))),
		StatePaused:   lipgloss.NewStyle().Foreground(fg),
		StateScanning: lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#06B"), lipgloss.Color("#5CF"))),

		DeviceConnected:    lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#080"), lipgloss.Color("#0E0"))),
		DeviceDisconnected: lipgloss.NewStyle().Foreground(fg),
		DevicePaused:       lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#A70"), lipgloss.Color("#FD0"))),

		PendingBadge: lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#A70"), lipgloss.Color("#FD0"))),

		ErrorBanner: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFF")).Background(lipgloss.Color("#C00")).Padding(0, 1),

		HelpKey:  lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#06B"), lipgloss.Color("#5CF"))),
		HelpDesc: lipgloss.NewStyle().Foreground(fg),

		Selected: lipgloss.NewStyle().Bold(true).
			Foreground(ld(lipgloss.Color("#000"), lipgloss.Color("#FFF"))).
			Background(ld(lipgloss.Color("#ADF"), lipgloss.Color("#348"))),

		FormLabel: lipgloss.NewStyle().Bold(true).Foreground(fg),

		SectionHeader: lipgloss.NewStyle().Bold(true).Foreground(fg),
		SectionHeaderFocused: lipgloss.NewStyle().Bold(true).
			Foreground(ld(lipgloss.Color("#000"), lipgloss.Color("#FFF"))).
			Background(ld(lipgloss.Color("#0AF"), lipgloss.Color("#36F"))),
	}
}
