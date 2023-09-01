// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/semaphore"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	folderFactories[config.FolderTypeSendOnly] = newSendOnlyFolder
}

type sendOnlyFolder struct {
	folder
}

func newSendOnlyFolder(model *model, fset *db.FileSet, ignores *ignore.Matcher, cfg config.FolderConfiguration, _ versioner.Versioner, evLogger events.Logger, ioLimiter *semaphore.Semaphore) service {
	f := &sendOnlyFolder{
		folder: newFolder(model, fset, ignores, cfg, evLogger, ioLimiter, nil),
	}
	f.folder.puller = f
	return f
}

func (*sendOnlyFolder) PullErrors() []FileError {
	return nil
}

// pull checks need for files that only differ by metadata (no changes on disk)
func (f *sendOnlyFolder) pull() (bool, error) {
	batch := db.NewFileInfoBatch(func(files []protocol.FileInfo) error {
		f.updateLocalsFromPulling(files)
		return nil
	})

	snap, err := f.dbSnapshot()
	if err != nil {
		return false, err
	}
	defer snap.Release()
	snap.WithNeed(protocol.LocalDeviceID, func(intf protocol.FileIntf) bool {
		batch.FlushIfFull()

		file := intf.(protocol.FileInfo)

		if f.ignores.ShouldIgnore(intf.FileName()) {
			file.SetIgnored()
			batch.Append(file)
			l.Debugln(f, "Handling ignored file", file)
			return true
		}

		curFile, ok := snap.Get(protocol.LocalDeviceID, intf.FileName())
		if !ok {
			if intf.IsInvalid() {
				// Global invalid file just exists for need accounting
				batch.Append(file)
			} else if intf.IsDeleted() {
				l.Debugln("Should never get a deleted file as needed when we don't have it")
				f.evLogger.Log(events.Failure, "got deleted file that doesn't exist locally as needed when pulling on send-only")
			}
			return true
		}

		if !file.IsEquivalentOptional(curFile, protocol.FileInfoComparison{
			ModTimeWindow:   f.modTimeWindow,
			IgnorePerms:     f.IgnorePerms,
			IgnoreOwnership: !f.SyncOwnership,
			IgnoreXattrs:    !f.SyncXattrs,
		}) {
			return true
		}

		batch.Append(file)
		l.Debugln(f, "Merging versions of identical file", file)

		return true
	})

	batch.Flush()

	return true, nil
}

func (f *sendOnlyFolder) Override() {
	f.doInSync(f.override)
}

func (f *sendOnlyFolder) override() error {
	l.Infoln("Overriding global state on folder", f.Description())

	f.setState(FolderScanning)
	defer f.setState(FolderIdle)

	batch := db.NewFileInfoBatch(func(files []protocol.FileInfo) error {
		f.updateLocalsFromScanning(files)
		return nil
	})
	snap, err := f.dbSnapshot()
	if err != nil {
		return err
	}
	defer snap.Release()
	snap.WithNeed(protocol.LocalDeviceID, func(fi protocol.FileIntf) bool {
		need := fi.(protocol.FileInfo)
		_ = batch.FlushIfFull()

		have, ok := snap.Get(protocol.LocalDeviceID, need.Name)
		// Don't override files that are in a bad state (ignored,
		// unsupported, must rescan, ...).
		if ok && have.IsInvalid() {
			return true
		}
		if !ok || have.Name != need.Name {
			// We are missing the file
			need.SetDeleted(f.shortID)
		} else {
			// We have the file, replace with our version
			have.Version = have.Version.Merge(need.Version).Update(f.shortID)
			need = have
		}
		need.Sequence = 0
		batch.Append(need)
		return true
	})
	return batch.Flush()
}
