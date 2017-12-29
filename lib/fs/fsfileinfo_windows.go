// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"os"
)

func (e fsFileInfo) Mode() FileMode {
	m := e.FileInfo.Mode()
	if m&os.ModeSymlink != 0 && e.Size() > 0 {
		// "Symlinks" with nonzero size are in fact "hard" links, such as
		// NTFS deduped files. Remove the symlink bit.
		m &^= os.ModeSymlink
	}
	return FileMode(m)
}
