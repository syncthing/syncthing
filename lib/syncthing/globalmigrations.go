// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"encoding/binary"
	"io"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	globalMigrationVersion = 1
	globalMigrationDBKey   = "globalMigrationVersion"
)

func globalMigration(ll *db.Lowlevel, cfg config.Wrapper) error {
	miscDB := db.NewMiscDataNamespace(ll)
	prevVersion, _, err := miscDB.Int64(globalMigrationDBKey)
	if err != nil {
		return err
	}

	if prevVersion >= globalMigrationVersion {
		return nil
	}

	if prevVersion < 1 {
		if err := encryptionTrailerSizeMigration(ll, cfg); err != nil {
			return err
		}
	}

	return miscDB.PutInt64(globalMigrationDBKey, globalMigrationVersion)
}

func encryptionTrailerSizeMigration(ll *db.Lowlevel, cfg config.Wrapper) error {
	encFolders := cfg.Folders()
	for folderID, folderCfg := range cfg.Folders() {
		if folderCfg.Type != config.FolderTypeReceiveEncrypted {
			delete(encFolders, folderID)
		}
	}
	if len(encFolders) == 0 {
		return nil
	}

	l.Infoln("Running global migration to fix encryption file sizes")

	// Trigger index re-transfer with fixed up sizes
	db.DropDeltaIndexIDs(ll)

	for folderID, folderCfg := range encFolders {
		fset, err := db.NewFileSet(folderID, ll)
		if err != nil {
			return err
		}
		snap, err := fset.Snapshot()
		if err != nil {
			return err
		}
		batch := db.NewFileInfoBatch(func(files []protocol.FileInfo) error {
			// As we can't touch the version, we need to first invalidate the
			// files, and then re-add the modified valid files
			invalidFiles := make([]protocol.FileInfo, len(files))
			for i, f := range files {
				f.SetUnsupported()
				invalidFiles[i] = f
			}
			fset.Update(protocol.LocalDeviceID, invalidFiles)
			fset.Update(protocol.LocalDeviceID, files)
			return nil
		})
		filesystem := folderCfg.Filesystem(fset)
		var innerErr error
		snap.WithHave(protocol.LocalDeviceID, func(intf protocol.FileIntf) bool {
			fi := intf.(protocol.FileInfo)
			size, err := sizeOfEncryptedTrailer(filesystem, fi.Name)
			if err != nil {
				// Best effort: If we fail to read a file, it will show as
				// locally changed on next scan.
				return true
			}
			fi.EncryptionTrailerSize = size
			batch.Append(fi)
			err = batch.FlushIfFull()
			if err != nil {
				innerErr = err
				return false
			}
			return true
		})
		snap.Release()
		if innerErr != nil {
			return innerErr
		}
		err = batch.Flush()
		if err != nil {
			return err
		}
	}

	return nil
}

// sizeOfEncryptedTrailer returns the size of the encrypted trailer on disk.
// This amount of bytes should be subtracted from the file size to get the
// original file size.
func sizeOfEncryptedTrailer(fs fs.Filesystem, name string) (int, error) {
	f, err := fs.Open(name)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if _, err := f.Seek(-4, io.SeekEnd); err != nil {
		return 0, err
	}
	var buf [4]byte
	if _, err := io.ReadFull(f, buf[:]); err != nil {
		return 0, err
	}
	// The stored size is the size of the encrypted data.
	size := int(binary.BigEndian.Uint32(buf[:]))
	// We add the size of the length word itself as well.
	return size + 4, nil
}
