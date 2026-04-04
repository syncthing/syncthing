// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tui

import (
	"fmt"
)

type confirmKind int

const (
	confirmRemoveFolder confirmKind = iota + 1
	confirmRemoveDevice
	confirmRestart
	confirmShutdown
	confirmOverride
	confirmRevert
)

type confirmModel struct {
	kind    confirmKind
	message string
	id      string // folder or device ID
}

func newConfirm(kind confirmKind, message, id string) confirmModel {
	return confirmModel{
		kind:    kind,
		message: message,
		id:      id,
	}
}

func (c *confirmModel) View(styles Styles) string {
	return fmt.Sprintf("\n  %s\n\n  %s",
		styles.ErrorBanner.Render(c.message),
		styles.Muted.Render("y: confirm  n/esc: cancel"),
	)
}
