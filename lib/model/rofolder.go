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
	"github.com/syncthing/syncthing/lib/fswatcher"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	folderFactories[config.FolderTypeSendOnly] = newSendOnlyFolder
}

type sendOnlyFolder struct {
	folder
}

func newSendOnlyFolder(model *Model, cfg config.FolderConfiguration, _ versioner.Versioner, _ fs.Filesystem, fsWatcher fswatcher.Service) service {
	return &sendOnlyFolder{folder: newFolder(model, cfg, fsWatcher)}
}

func (f *sendOnlyFolder) Serve() {
	l.Debugln(f, "starting")
	defer l.Debugln(f, "exiting")

	defer func() {
		f.scan.timer.Stop()
	}()

	for {
		select {
		case <-f.ctx.Done():
			return

		case <-f.scan.timer.C:
			l.Debugln(f, "Scanning subdirectories")
			err := f.scanSubdirs(nil)

			select {
			case <-f.initialScanFinished:
			default:
				status := "Completed"
				if err != nil {
					status = "Failed"
				}
				l.Infoln(status, "initial scan (ro) of", f.Description())
				close(f.initialScanFinished)
			}

			if f.scan.HasNoInterval() {
				continue
			}

			f.scan.Reschedule()

		case req := <-f.scan.now:
			req.err <- f.scanSubdirs(req.subdirs)

		case next := <-f.scan.delay:
			f.scan.timer.Reset(next)

		case fsEvents := <-f.fsWatcherChan:
			l.Debugln(f, "filesystem notification rescan")
			f.scanSubdirs(fsEvents)
		}
	}
}

func (f *sendOnlyFolder) String() string {
	return fmt.Sprintf("sendOnlyFolder/%s@%p", f.folderID, f)
}
