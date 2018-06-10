// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tray

import (
	"github.com/syncthing/syncthing/lib/tray/menu"
	"errors"
)

type Tray interface {
	SetTooltip(string) error
	SetOnLeftClick(func())
	SetOnRightClick(func())
	SetOnDoubleClick(func())
	ShowMenu()
	SetMenuCreationCallback(func() []menu.Item)
	Serve()
	Stop()
}

func New() (Tray, error) {
	return newTray()
}

var ErrNotSupported = errors.New("not supported")
