// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

func (e fsFileInfo) Owner() int {
	return -1
}

func (e fsFileInfo) Group() int {
	return -1
}
