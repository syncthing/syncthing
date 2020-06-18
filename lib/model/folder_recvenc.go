// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	// A receiveEncrypted folder behaves just like send-receive folder,
	// except for scanning which is handled in folder.
	folderFactories[config.FolderTypeReceiveEncrypted] = newReceiveEncryptedFolder
}

type receiveEncryptedFolder struct {
	*sendReceiveFolder
}

func newReceiveEncryptedFolder(model *model, fset *db.FileSet, ignores *ignore.Matcher, cfg config.FolderConfiguration, ver versioner.Versioner, fs fs.Filesystem, evLogger events.Logger, ioLimiter *byteSemaphore) service {
	return &receiveOnlyFolder{newSendReceiveFolder(model, fset, ignores, cfg, ver, fs, evLogger, ioLimiter).(*sendReceiveFolder)}
}

func (f *receiveOnlyFolder) CleanEnc() {
	f.doInSync(func() error { f.revert(); return nil })
}

func (f *receiveOnlyFolder) cleanEnc() {
	l.Infof("Cleaning not encrypted items from folder %v", f.Description)

	f.setState(FolderScanning)
	defer f.setState(FolderIdle)

	batch := newFileInfoBatch(func(fs []protocol.FileInfo) error {
		f.updateLocalsFromScanning(fs)
		return nil
	})

	snap := f.fset.Snapshot()
	defer snap.Release()
	var iterErr error
	snap.WithHaveTruncated(protocol.LocalDeviceID, func(intf protocol.FileIntf) bool {
		if iterErr = batch.flushIfFull(); iterErr != nil {
			return false
		}

		fit := intf.(db.FileInfoTruncated)
		if !fit.IsReceiveOnlyChanged() || intf.IsDeleted() || protocol.IsEncryptedPath(fit.Name) {
			return true
		}

		if err := f.fs.RemoveAll(fit.Name); err != nil && !fs.IsNotExist(err) {
			f.newScanError(fit.Name, fmt.Errorf("cleaning not encrypted item: %w", err))
		}

		fi := fit.ConvertToDeletedFileInfo(f.shortID)
		fi.LocalFlags &^= protocol.FlagLocalReceiveOnly
		batch.append(fi)

		return true
	})

	if iterErr == nil {
		iterErr = batch.flush()
	}
	if iterErr != nil {
		l.Infoln("Failed to clean not encrypted items:", iterErr)
	}
}
