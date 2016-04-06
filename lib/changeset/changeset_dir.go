// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

// When chmodding, retain these bits if they are set
const retainBits = os.ModeSetuid | os.ModeSetgid | os.ModeSticky

func (c *ChangeSet) writeDir(d protocol.FileInfo) *opError {
	realPath := filepath.Join(c.rootPath, d.Name)

	if d.Flags&protocol.FlagNoPermBits != 0 {
		// If the directory already exists we have nothing further to do, as
		// we should not touch the permission bits.
		if info, err := c.filesystem.Lstat(realPath); err == nil && info.IsDir() {
			return nil
		}

		// Ignore permissions is set, so use a default set of bits (filtered
		// by umask by the OS at create time).
		mode := os.FileMode(0777)
		if err := c.filesystem.Mkdir(realPath, mode); err != nil {
			return &opError{file: d.Name, op: "writeDir Mkdir", err: err}
		}
		return nil
	}

	// Use the permission bits from the FileInfo.
	mode := os.FileMode(d.Flags & 0777)

	// Check if the directory already exists and if so just set the
	// permissions. If bits from the set in retainBits are set we make sure
	// they are kept set.
	if info, err := c.filesystem.Lstat(realPath); err == nil && info.IsDir() {
		mode = mode | info.Mode()&retainBits
		if err = c.filesystem.Chmod(realPath, mode); err != nil {
			return &opError{file: d.Name, op: "writeDir Chmod", err: err}
		}
		return nil
	}

	// Create the missing directory and set the permission bits.
	if err := c.filesystem.Mkdir(realPath, mode); err != nil {
		return &opError{file: d.Name, op: "writeDir Mkdir", err: err}
	}
	if err := c.filesystem.Chmod(realPath, mode); err != nil {
		return &opError{file: d.Name, op: "writeDir Chmod", err: err}
	}

	return nil
}

func (c *ChangeSet) deleteDir(d protocol.FileInfo) *opError {
	realPath := filepath.Join(c.rootPath, d.Name)
	if _, err := c.filesystem.Lstat(realPath); err != nil {
		// Things that we can't stat don't exist
		return nil
	}
	if err := osutil.InWritableDir(c.filesystem.Remove, realPath); err != nil {
		return &opError{file: d.Name, op: "deleteDir Remove", err: err}
	}

	return nil
}
