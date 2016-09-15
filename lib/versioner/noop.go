// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package versioner

import (
	"github.com/syncthing/syncthing/lib/osutil"
	"os"
)

func init() {
	// Register the constructor for this type of versioner with the name "noop"
	Factories["noop"] = NewNoop
}

type Noop struct {
	folderPath string
}

func NewNoop(folderID, folderPath string, params map[string]string) Versioner {
	n := Noop{
		folderPath: folderPath,
	}

	l.Debugf("instantiated %#v", n)
	return n
}

func (v Noop) Remove(oldPath string) error {
	return os.Remove(oldPath)
}

func (v Noop) Replace(oldPath, newPath string) error {
	return osutil.TryRename(oldPath, newPath)
}

// Archive moves the named file away to a version archive. If this function
// returns nil, the named file does not exist any more (has been archived).
func (v Noop) Archive(filePath string) error {
	// placeholder while vip
	return nil
}
