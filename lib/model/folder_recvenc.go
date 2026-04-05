// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/semaphore"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	folderFactories[config.FolderTypeReceiveEncrypted] = newReceiveEncryptedFolder
}

type receiveEncryptedFolder struct {
	*sendReceiveFolder
}

func newReceiveEncryptedFolder(model *model, ignores *ignore.Matcher, cfg config.FolderConfiguration, ver versioner.Versioner, evLogger events.Logger, ioLimiter *semaphore.Semaphore) service {
	f := &receiveEncryptedFolder{newSendReceiveFolder(model, ignores, cfg, ver, evLogger, ioLimiter).(*sendReceiveFolder)}
	f.localFlags = protocol.FlagLocalReceiveOnly // gets propagated to the scanner, and set on locally changed files
	return f
}

func (f *receiveEncryptedFolder) Revert() {
	f.doInSync(f.revert)
}

func (f *receiveEncryptedFolder) revert(ctx context.Context) error {
	f.sl.InfoContext(ctx, "Reverting unexpected items")

	f.setState(FolderScanning)
	defer f.setState(FolderIdle)

	batch := NewFileInfoBatch(func(fs []protocol.FileInfo) error {
		f.updateLocalsFromScanning(fs)
		return nil
	})

	var dirs []string
	for fi, err := range itererr.Zip(f.db.AllLocalFiles(f.folderID, protocol.LocalDeviceID)) {
		if err != nil {
			return err
		}
		if err := batch.FlushIfFull(); err != nil {
			return err
		}

		if !fi.IsReceiveOnlyChanged() || fi.IsDeleted() {
			continue
		}

		if fi.IsDirectory() {
			dirs = append(dirs, fi.Name)
			continue
		}

		if err := f.inWritableDir(f.mtimefs.Remove, fi.Name); err != nil && !fs.IsNotExist(err) {
			f.newScanError(fi.Name, fmt.Errorf("deleting unexpected item: %w", err))
		}

		fi.SetDeleted(f.shortID)
		// Set version to zero, such that we pull the global version in case
		// this is a valid filename that was erroneously changed locally.
		// Should already be zero from scanning, but lets be safe.
		fi.Version = protocol.Vector{}
		// Purposely not removing FlagLocalReceiveOnly as the deleted
		// item should still not be sent in index updates. However being
		// deleted, it will not show up as an unexpected file in the UI
		// anymore.
		batch.Append(fi)
	}

	f.revertHandleDirs(ctx, dirs)

	if err := batch.Flush(); err != nil {
		return err
	}

	// We might need to pull items if the local changes were on valid, global files.
	f.SchedulePull()

	return nil
}

func (f *receiveEncryptedFolder) revertHandleDirs(ctx context.Context, dirs []string) {
	if len(dirs) == 0 {
		return
	}

	scanChan := make(chan string)
	go f.pullScannerRoutine(ctx, scanChan)
	defer close(scanChan)

	slices.SortFunc(dirs, func(a, b string) int {
		return strings.Compare(b, a)
	})
	for _, dir := range dirs {
		if err := f.deleteDirOnDisk(dir, scanChan); err != nil {
			f.newScanError(dir, fmt.Errorf("deleting unexpected dir: %w", err))
		}
		scanChan <- dir
	}
}
