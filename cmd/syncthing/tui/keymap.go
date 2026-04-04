// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"charm.land/bubbles/v2/key"
)

type keyMap struct {
	Quit key.Binding
	Help key.Binding

	// Tab switching
	TabFolders key.Binding
	TabDevices key.Binding
	TabEvents  key.Binding
	NextTab    key.Binding
	PrevTab    key.Binding

	// List navigation
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Back     key.Binding
	PageUp   key.Binding
	PageDown key.Binding

	// Actions
	Add           key.Binding
	Edit          key.Binding
	Pause         key.Binding
	Scan          key.Binding
	Remove        key.Binding
	RestartDaemon key.Binding
	ShutDown      key.Binding
	Share         key.Binding
	Override      key.Binding
	Revert        key.Binding
	ShowID        key.Binding
	ClearErrors   key.Binding
	Logs          key.Binding

	// Confirm dialog
	ConfirmYes key.Binding
	ConfirmNo  key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		TabFolders: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "folders tab"),
		),
		TabDevices: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "devices tab"),
		),
		TabEvents: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "events tab"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev tab"),
		),

		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k/up", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/down", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "expand/collapse"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "backspace"),
			key.WithHelp("esc", "back"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),

		Add: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "add"),
		),
		Edit: key.NewBinding(
			key.WithKeys("E"),
			key.WithHelp("E", "edit"),
		),
		Pause: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "pause/resume"),
		),
		Scan: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "scan"),
		),
		Remove: key.NewBinding(
			key.WithKeys("delete", "x"),
			key.WithHelp("x/del", "remove"),
		),
		RestartDaemon: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "restart daemon"),
		),
		ShutDown: key.NewBinding(
			key.WithKeys("Q"),
			key.WithHelp("Q", "shut down daemon"),
		),
		Share: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "share to device"),
		),
		Override: key.NewBinding(
			key.WithKeys("O"),
			key.WithHelp("O", "override changes"),
		),
		Revert: key.NewBinding(
			key.WithKeys("V"),
			key.WithHelp("V", "revert changes"),
		),
		ShowID: key.NewBinding(
			key.WithKeys("I"),
			key.WithHelp("I", "show device ID + QR"),
		),
		ClearErrors: key.NewBinding(
			key.WithKeys("C"),
			key.WithHelp("C", "clear errors"),
		),
		Logs: key.NewBinding(
			key.WithKeys("L"),
			key.WithHelp("L", "system logs"),
		),

		ConfirmYes: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "yes"),
		),
		ConfirmNo: key.NewBinding(
			key.WithKeys("n", "esc"),
			key.WithHelp("n/esc", "no"),
		),
	}
}

// ShortHelp returns the key bindings shown in the compact help.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit, k.ShutDown, k.ShowID, k.ClearErrors}
}

// FullHelp returns the full set of key bindings for the help overlay.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.TabFolders, k.TabDevices, k.TabEvents, k.NextTab, k.PrevTab},
		{k.Up, k.Down, k.Enter, k.Back, k.PageUp, k.PageDown},
		{k.Add, k.Edit, k.Pause, k.Scan, k.Remove, k.Share, k.Override, k.Revert, k.ClearErrors},
		{k.ShowID, k.Logs, k.RestartDaemon, k.ShutDown, k.Help, k.Quit},
	}
}
