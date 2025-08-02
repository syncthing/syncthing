// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"slices"
	"strings"
	"time"

	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/semaphore"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	folderFactories[config.FolderTypeReceiveOnly] = newReceiveOnlyFolder
}

/*
receiveOnlyFolder is a folder that does not propagate local changes outward.
It does this by the following general mechanism (not all of which is
implemented in this file):

  - Local changes are scanned and versioned as usual, but get the
    FlagLocalReceiveOnly bit set.

  - When changes are sent to the cluster this bit gets converted to the
    Invalid bit (like all other local flags, currently) and also the Version
    gets set to the empty version. The reason for clearing the Version is to
    ensure that other devices will not consider themselves out of date due to
    our change.

  - The database layer accounts sizes per flag bit, so we can know how many
    files have been changed locally. We use this to trigger a "Revert" option
    on the folder when the amount of locally changed data is nonzero.

  - To revert we take the files which have changed and reset their version
    counter down to zero. The next pull will replace our changed version with
    the globally latest. As this is a user-initiated operation we do not cause
    conflict copies when reverting.

  - When pulling normally (i.e., not in the revert case) with local changes,
    normal conflict resolution will apply. Conflict copies will be created,
    but not propagated outwards (because receive only, right).

Implementation wise a receiveOnlyFolder is just a sendReceiveFolder that
sets an extra bit on local changes and has a Revert method.
*/
type receiveOnlyFolder struct {
	*sendReceiveFolder
}

func newReceiveOnlyFolder(model *model, ignores *ignore.Matcher, cfg config.FolderConfiguration, ver versioner.Versioner, evLogger events.Logger, ioLimiter *semaphore.Semaphore) service {
	sr := newSendReceiveFolder(model, ignores, cfg, ver, evLogger, ioLimiter).(*sendReceiveFolder)
	sr.localFlags = protocol.FlagLocalReceiveOnly // gets propagated to the scanner, and set on locally changed files
	return &receiveOnlyFolder{sr}
}

func (f *receiveOnlyFolder) Revert() {
	f.doInSync(f.revert)
}

func (f *receiveOnlyFolder) revert() error {
	f.sl.Info("Reverting folder")

	f.setState(FolderScanning)
	defer f.setState(FolderIdle)

	scanChan := make(chan string)
	go f.pullScannerRoutine(scanChan)
	defer close(scanChan)

	delQueue := &deleteQueue{
		handler:  f, // for the deleteItemOnDisk and deleteDirOnDisk methods
		ignores:  f.ignores,
		scanChan: scanChan,
	}

	batch := NewFileInfoBatch(func(files []protocol.FileInfo) error {
		f.updateLocalsFromScanning(files)
		return nil
	})

	for fi, err := range itererr.Zip(f.db.AllLocalFiles(f.folderID, protocol.LocalDeviceID)) {
		if err != nil {
			return err
		}
		if !fi.IsReceiveOnlyChanged() {
			// We're only interested in files that have changed locally in
			// receive only mode.
			continue
		}

		fi.LocalFlags &^= protocol.FlagLocalReceiveOnly

		switch gf, ok, err := f.db.GetGlobalFile(f.folderID, fi.Name); {
		case err != nil:
			return err
		case !ok:
			msg := "Unexpectedly missing global file that we have locally"
			l.Debugf("%v revert: %v: %v", f, msg, fi.Name)
			f.evLogger.Log(events.Failure, msg)
			continue
		case gf.IsReceiveOnlyChanged():
			// The global file is our own. A revert then means to delete it.
			// We'll delete files directly, directories get queued and
			// handled below.
			if fi.Deleted {
				fi.Version = protocol.Vector{} // if this file ever resurfaces anywhere we want our delete to be strictly older
				break
			}
			l.Debugf("Revert: deleting %s: %v\n", fi.Name, err)
			handled, err := delQueue.handle(fi)
			if err != nil {
				continue
			}
			if !handled {
				continue
			}
			fi.SetDeleted(f.shortID)
			fi.Version = protocol.Vector{} // if this file ever resurfaces anywhere we want our delete to be strictly older
		case gf.IsEquivalentOptional(fi, protocol.FileInfoComparison{
			ModTimeWindow:   f.modTimeWindow,
			IgnoreFlags:     protocol.FlagLocalReceiveOnly,
			IgnoreOwnership: !f.SyncOwnership,
			IgnoreXattrs:    !f.SyncXattrs,
		}):
			// What we have locally is equivalent to the global file.
			fi = gf
		default:
			// Revert means to throw away our local changes. We reset the
			// version to the empty vector, which is strictly older than any
			// other existing version. It is not in conflict with anything,
			// either, so we will not create a conflict copy of our local
			// changes.
			fi.Version = protocol.Vector{}
		}

		batch.Append(fi)
		_ = batch.FlushIfFull()
	}
	if err := batch.Flush(); err != nil {
		return err
	}

	// Handle any queued directories
	deleted, err := delQueue.flush()
	if err != nil {
		f.sl.Warn("Failed to revert directories", slogutil.Error(err))
	}
	now := time.Now()
	for _, dir := range deleted {
		batch.Append(protocol.FileInfo{
			Name:       dir,
			Type:       protocol.FileInfoTypeDirectory,
			ModifiedS:  now.Unix(),
			ModifiedBy: f.shortID,
			Deleted:    true,
			Version:    protocol.Vector{},
		})
	}
	_ = batch.Flush()

	// We will likely have changed our local index, but that won't trigger a
	// pull by itself. Make sure we schedule one so that we start
	// downloading files.
	f.SchedulePull()

	return nil
}

// deleteQueue handles deletes by delegating to a handler and queuing
// directories for last.
type deleteQueue struct {
	handler interface {
		deleteItemOnDisk(item protocol.FileInfo, scanChan chan<- string) error
		deleteDirOnDisk(dir string, scanChan chan<- string) error
	}
	ignores  *ignore.Matcher
	dirs     []string
	scanChan chan<- string
}

func (q *deleteQueue) handle(fi protocol.FileInfo) (bool, error) {
	// Things that are ignored but not marked deletable are not processed.
	ign := q.ignores.Match(fi.Name)
	if ign.IsIgnored() && !ign.IsDeletable() {
		return false, nil
	}

	// Directories are queued for later processing.
	if fi.IsDirectory() {
		q.dirs = append(q.dirs, fi.Name)
		return false, nil
	}

	// Kill it.
	err := q.handler.deleteItemOnDisk(fi, q.scanChan)
	return true, err
}

func (q *deleteQueue) flush() ([]string, error) {
	// Process directories from the leaves inward.
	slices.SortFunc(q.dirs, func(a, b string) int {
		return strings.Compare(b, a)
	})

	var firstError error
	var deleted []string

	for _, dir := range q.dirs {
		if err := q.handler.deleteDirOnDisk(dir, q.scanChan); err == nil {
			deleted = append(deleted, dir)
		} else if firstError == nil {
			firstError = err
		}
	}

	return deleted, firstError
}
