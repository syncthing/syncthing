// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"

	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/config"
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
	*folder
}

func newSendOnlyFolder(model *model, ignores *ignore.Matcher, cfg config.FolderConfiguration, _ versioner.Versioner, evLogger events.Logger, ioLimiter *semaphore.Semaphore) service {
	f := &sendOnlyFolder{
		folder: newFolder(model, ignores, cfg, evLogger, ioLimiter, nil),
	}
	f.puller = f
	return f
}

func (*sendOnlyFolder) PullErrors() []FileError {
	return nil
}

// pull checks need for files that only differ by metadata (no changes on disk)
func (f *sendOnlyFolder) pull(ctx context.Context) (bool, error) {
	batch := NewFileInfoBatch(func(files []protocol.FileInfo) error {
		f.updateLocalsFromPulling(files)
		return nil
	})

	for file, err := range itererr.Zip(f.db.AllNeededGlobalFiles(f.folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 0, 0)) {
		if err != nil {
			return false, err
		}
		if err := batch.FlushIfFull(); err != nil {
			return false, err
		}

		if f.ignores.Match(file.FileName()).IsIgnored() {
			file.SetIgnored()
			batch.Append(file)
			l.Debugln(f, "Handling ignored file", file)
			continue
		}

		curFile, ok, err := f.db.GetDeviceFile(f.folderID, protocol.LocalDeviceID, file.FileName())
		if err != nil {
			return false, err
		}
		if !ok {
			if file.IsInvalid() || file.IsDeleted() {
				// Accept the file for accounting purposes
				batch.Append(file)
			}
			continue
		}

		if !file.IsEquivalentOptional(curFile, protocol.FileInfoComparison{
			ModTimeWindow:   f.modTimeWindow,
			IgnorePerms:     f.IgnorePerms,
			IgnoreOwnership: !f.SyncOwnership,
			IgnoreXattrs:    !f.SyncXattrs,
		}) {
			continue
		}

		batch.Append(file)
		l.Debugln(f, "Merging versions of identical file", file)
	}

	batch.Flush()

	return true, nil
}

func (f *sendOnlyFolder) Override() {
	f.doInSync(f.override)
}

func (f *sendOnlyFolder) override(ctx context.Context) error {
	f.sl.InfoContext(ctx, "Overriding global state ")

	f.setState(FolderScanning)
	defer f.setState(FolderIdle)

	batch := NewFileInfoBatch(func(files []protocol.FileInfo) error {
		f.updateLocalsFromScanning(files)
		return nil
	})

	for need, err := range itererr.Zip(f.db.AllNeededGlobalFiles(f.folderID, protocol.LocalDeviceID, config.PullOrderAlphabetic, 0, 0)) {
		if err != nil {
			return err
		}
		if err := batch.FlushIfFull(); err != nil {
			return err
		}

		have, haveOk, err := f.db.GetDeviceFile(f.folderID, protocol.LocalDeviceID, need.Name)
		if err != nil {
			return err
		}

		// Don't override files that are in a bad state (ignored,
		// unsupported, must rescan, ...).
		if haveOk && have.IsInvalid() {
			continue
		}

		if !haveOk || have.Name != need.Name {
			// We are missing the file
			need.SetDeleted(f.shortID)
		} else {
			// We have the file, replace with our version
			have.Version = have.Version.Merge(need.Version).Update(f.shortID)
			need = have
		}
		need.Sequence = 0
		batch.Append(need)
	}
	return batch.Flush()
}
