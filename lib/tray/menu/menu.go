// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package menu

type Type uintptr
type State uintptr

const (
	TypeSubMenu   Type = 0x00000010
	TypeSeparator Type = 0x00000800
	TypeRadio     Type = 0x00000200

	StateDisabled  State = 0x00000002
	StateGrayed    State = 0x00000001
	StateChecked   State = 0x00000008
	StateDefault   State = 0x00001000
	StateHighlight State = 0x00000080
)

type Item struct {
	Name     string
	Type     Type
	State    State
	OnClick  func()
	Children []Item
}
