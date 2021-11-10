// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"sort"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/util"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	folderFactories[config.FolderTypeReceiveEncrypted] = newReceiveEncryptedFolder
}

type receiveEncryptedFolder struct {
	*sendReceiveFolder
}

func newReceiveEncryptedFolder(model *model, fset *db.FileSet, ignores *ignore.Matcher, cfg config.FolderConfiguration, ver versioner.Versioner, evLogger events.Logger, ioLimiter *util.Semaphore) service {
	f := &receiveEncryptedFolder{newSendReceiveFolder(model, fset, ignores, cfg, ver, evLogger, ioLimiter).(*sendReceiveFolder)}
	f.localFlags = protocol.FlagLocalReceiveOnly // gets propagated to the scanner, and set on locally changed files
	return f
}

func (f *receiveEncryptedFolder) Revert() {
	f.doInSync(f.revert)
}

func (f *receiveEncryptedFolder) revert() error {
	l.Infof("Reverting unexpected items in folder %v (receive-encrypted)", f.Description())

	f.setState(FolderScanning)
	defer f.setState(FolderIdle)

	batch := db.NewFileInfoBatch(func(fs []protocol.FileInfo) error {
		f.updateLocalsFromScanning(fs)
		return nil
	})

	snap, err := f.dbSnapshot()
	if err != nil {
		return err
	}
	defer snap.Release()
	var iterErr error
	var dirs []string
	snap.WithHaveTruncated(protocol.LocalDeviceID, func(intf protocol.FileIntf) bool {
		if iterErr = batch.FlushIfFull(); iterErr != nil {
			return false
		}

		fit := intf.(db.FileInfoTruncated)
		if !fit.IsReceiveOnlyChanged() || intf.IsDeleted() {
			return true
		}

		if fit.IsDirectory() {
			dirs = append(dirs, fit.Name)
			return true
		}

		if err := f.inWritableDir(f.mtimefs.Remove, fit.Name); err != nil && !fs.IsNotExist(err) {
			f.newScanError(fit.Name, fmt.Errorf("deleting unexpected item: %w", err))
		}

		fi := fit.ConvertToDeletedFileInfo(f.shortID)
		// Set version to zero, such that we pull the global version in case
		// this is a valid filename that was erroneously changed locally.
		// Should already be zero from scanning, but lets be safe.
		fi.Version = protocol.Vector{}
		// Purposely not removing FlagLocalReceiveOnly as the deleted
		// item should still not be sent in index updates. However being
		// deleted, it will not show up as an unexpected file in the UI
		// anymore.
		batch.Append(fi)

		return true
	})

	f.revertHandleDirs(dirs, snap)

	if iterErr != nil {
		return iterErr
	}
	if err := batch.Flush(); err != nil {
		return err
	}

	// We might need to pull items if the local changes were on valid, global files.
	f.SchedulePull()

	return nil
}

func (f *receiveEncryptedFolder) revertHandleDirs(dirs []string, snap *db.Snapshot) {
	if len(dirs) == 0 {
		return
	}

	scanChan := make(chan string)
	go f.pullScannerRoutine(scanChan)
	defer close(scanChan)

	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		if err := f.deleteDirOnDisk(dir, snap, scanChan); err != nil {
			f.newScanError(dir, fmt.Errorf("deleting unexpected dir: %w", err))
		}
		scanChan <- dir
	}
}
