// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

func (c *ChangeSet) writeSymlink(f protocol.FileInfo) *opError {
	realPath := filepath.Join(c.rootPath, f.Name)

	if len(f.Blocks) != 1 {
		// This is a really weird symlink.
		return &opError{file: f.Name, op: "writeSymlink", err: fmt.Errorf("len(blocks) %d != 1 for symlink", len(f.Blocks))}
	}

	// The size is actually the length of the target name.
	buf := make([]byte, f.Blocks[0].Size)

	// Try to reuse the block from somewhere local.
	err := c.localRequester.Request(f.Name, 0, f.Blocks[0].Hash, buf)
	if err != nil {
		// We got an error from the local source, try to request it from the
		// network instead.
		resp := c.networkRequester.Request(f.Name, 0, f.Blocks[0].Hash, int(f.Blocks[0].Size))
		err = resp.Error()
		buf = resp.Bytes()
		defer resp.Close()
	}
	if err != nil {
		// We failed to acquire the block.
		return &opError{file: f.Name, op: "writeSymlink pull", err: err}
	}

	tt := fs.LinkTargetFile
	if f.IsDirectory() {
		tt = fs.LinkTargetDirectory
	}

	err = osutil.InWritableDir(func(realPath string) error {
		c.filesystem.Remove(realPath)
		return c.filesystem.CreateSymlink(realPath, string(buf), tt)
	}, realPath)
	if err != nil {
		return &opError{file: f.Name, op: "writeSymlink create", err: err}
	}

	return nil
}

func (c *ChangeSet) deleteSymlink(f protocol.FileInfo) *opError {
	realPath := filepath.Join(c.rootPath, f.Name)
	if _, err := c.filesystem.Lstat(realPath); os.IsNotExist(err) {
		return nil
	}
	if err := osutil.InWritableDir(c.filesystem.Remove, realPath); err != nil {
		return &opError{file: f.Name, op: "deleteSymlink remove", err: err}
	}

	return nil
}
