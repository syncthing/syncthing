// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
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

	if f.FSWatcherEnabled && f.CheckHealth() == nil {
		f.startWatch()
	}

	for {
		select {
		case <-f.ctx.Done():
			return

		case <-f.pullScheduled:
			f.pull()

		case <-f.restartWatchChan:
			f.restartWatch()

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

func (f *sendOnlyFolder) PullErrors() []FileError {
	return nil
}

// pull checks need for files that only differ by metadata (no changes on disk)
func (f *sendOnlyFolder) pull() {
	select {
	case <-f.initialScanFinished:
	default:
		// Once the initial scan finished, a pull will be scheduled
		return
	}

	f.model.fmut.RLock()
	folderFiles := f.model.folderFiles[f.folderID]
	ignores := f.model.folderIgnores[f.folderID]
	f.model.fmut.RUnlock()

	batch := make([]protocol.FileInfo, 0, maxBatchSizeFiles)
	batchSizeBytes := 0

	folderFiles.WithNeed(protocol.LocalDeviceID, func(intf db.FileIntf) bool {
		if len(batch) == maxBatchSizeFiles || batchSizeBytes > maxBatchSizeBytes {
			f.model.updateLocalsFromPulling(f.folderID, batch)
			batch = batch[:0]
			batchSizeBytes = 0
		}

		if ignores.ShouldIgnore(intf.FileName()) {
			file := intf.(protocol.FileInfo)
			file.Invalidate(f.shortID)
			batch = append(batch, file)
			batchSizeBytes += file.ProtoSize()
			l.Debugln(f, "Handling ignored file", file)
			return true
		}

		curFile, ok := f.model.CurrentFolderFile(f.folderID, intf.FileName())
		if !ok {
			if intf.IsDeleted() {
				panic("Should never get a deleted file as needed when we don't have it")
			}
			return true
		}

		file := intf.(protocol.FileInfo)
		if !file.IsEquivalent(curFile, f.IgnorePerms, false) {
			return true
		}

		file.Version = file.Version.Merge(curFile.Version)
		batch = append(batch, file)
		batchSizeBytes += file.ProtoSize()
		l.Debugln(f, "Merging versions of identical file", file)

		return true
	})

	if len(batch) > 0 {
		f.model.updateLocalsFromPulling(f.folderID, batch)
	}
}
