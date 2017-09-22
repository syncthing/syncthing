// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	folderFactories[config.FolderTypeSendOnly] = newSendOnlyFolder
}

type sendOnlyFolder struct {
	folder
}

func newSendOnlyFolder(model *Model, cfg config.FolderConfiguration, _ versioner.Versioner, _ fs.Filesystem) service {
	return &sendOnlyFolder{folder: newFolder(model, cfg)}
}

func (f *sendOnlyFolder) Serve() {
	l.Debugln(f, "starting")
	defer l.Debugln(f, "exiting")

	defer func() {
		f.scan.timer.Stop()
	}()

	if f.FSWatcherEnabled {
		f.startWatcher()
	}

	for {
		select {
		case <-f.ctx.Done():
			return

		case <-f.ignoresUpdated:
			if f.FSWatcherEnabled {
				f.restartWatcher()
			}

		case <-f.scan.timer.C:
			l.Debugln(f, "Scanning subdirectories")
			f.scanTimerFired()

		case req := <-f.scan.now:
			req.err <- f.scanSubdirs(req.subdirs)

		case next := <-f.scan.delay:
			f.scan.timer.Reset(next)

		case fsEvents := <-f.watchChan:
			l.Debugln(f, "filesystem notification rescan")
			f.scanSubdirs(fsEvents)
		}
	}
}

func (f *sendOnlyFolder) String() string {
	return fmt.Sprintf("sendOnlyFolder/%s@%p", f.folderID, f)
}
