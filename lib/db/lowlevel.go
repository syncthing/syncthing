// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"encoding/binary"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/protocol"
)

// Lowlevel is the lowest level database interface. It has a very simple
// purpose: hold the actual backend database, and the in-memory state
// that belong to that database. In the same way that a single on disk
// database can only be opened once, there should be only one Lowlevel for
// any given backend.
type Lowlevel struct {
	backend.Backend
	folderIdx *smallIndex
	deviceIdx *smallIndex
	keyer     keyer
}

func NewLowlevel(backend backend.Backend) *Lowlevel {
	db := &Lowlevel{
		Backend:   backend,
		folderIdx: newSmallIndex(backend, []byte{KeyTypeFolderIdx}),
		deviceIdx: newSmallIndex(backend, []byte{KeyTypeDeviceIdx}),
	}
	db.keyer = newDefaultKeyer(db.folderIdx, db.deviceIdx)
	return db
}

// ListFolders returns the list of folders currently in the database
func (db *Lowlevel) ListFolders() []string {
	return db.folderIdx.Values()
}

// updateRemoteFiles adds a list of fileinfos to the database and updates the
// global versionlist and metadata.
func (db *Lowlevel) updateRemoteFiles(folder, device []byte, fs []protocol.FileInfo, meta *metadataTracker) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var dk, gk, keyBuf []byte
	devID := protocol.DeviceIDFromBytes(device)
	for _, f := range fs {
		name := []byte(f.Name)
		dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, device, name)
		if err != nil {
			return err
		}

		ef, ok, err := t.getFileTrunc(dk, true)
		if err != nil {
			return err
		}
		if ok && unchanged(f, ef) {
			continue
		}

		if ok {
			meta.removeFile(devID, ef)
		}
		meta.addFile(devID, f)

		l.Debugf("insert; folder=%q device=%v %v", folder, devID, f)
		if err := t.Put(dk, mustMarshal(&f)); err != nil {
			return err
		}

		gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
		if err != nil {
			return err
		}
		keyBuf, _, err = t.updateGlobal(gk, keyBuf, folder, device, f, meta)
		if err != nil {
			return err
		}

		if err := t.Checkpoint(); err != nil {
			return err
		}
	}

	return t.commit()
}

// updateLocalFiles adds fileinfos to the db, and updates the global versionlist,
// metadata, sequence and blockmap buckets.
func (db *Lowlevel) updateLocalFiles(folder []byte, fs []protocol.FileInfo, meta *metadataTracker) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var dk, gk, keyBuf []byte
	blockBuf := make([]byte, 4)
	for _, f := range fs {
		name := []byte(f.Name)
		dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, protocol.LocalDeviceID[:], name)
		if err != nil {
			return err
		}

		ef, ok, err := t.getFileByKey(dk)
		if err != nil {
			return err
		}
		if ok && unchanged(f, ef) {
			continue
		}

		if ok {
			if !ef.IsDirectory() && !ef.IsDeleted() && !ef.IsInvalid() {
				for _, block := range ef.Blocks {
					keyBuf, err = db.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
					if err != nil {
						return err
					}
					if err := t.Delete(keyBuf); err != nil {
						return err
					}
				}
			}

			keyBuf, err = db.keyer.GenerateSequenceKey(keyBuf, folder, ef.SequenceNo())
			if err != nil {
				return err
			}
			if err := t.Delete(keyBuf); err != nil {
				return err
			}
			l.Debugf("removing sequence; folder=%q sequence=%v %v", folder, ef.SequenceNo(), ef.FileName())
		}

		f.Sequence = meta.nextLocalSeq()

		if ok {
			meta.removeFile(protocol.LocalDeviceID, ef)
		}
		meta.addFile(protocol.LocalDeviceID, f)

		l.Debugf("insert (local); folder=%q %v", folder, f)
		if err := t.Put(dk, mustMarshal(&f)); err != nil {
			return err
		}

		gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, []byte(f.Name))
		if err != nil {
			return err
		}
		keyBuf, _, err = t.updateGlobal(gk, keyBuf, folder, protocol.LocalDeviceID[:], f, meta)
		if err != nil {
			return err
		}

		keyBuf, err = db.keyer.GenerateSequenceKey(keyBuf, folder, f.Sequence)
		if err != nil {
			return err
		}
		if err := t.Put(keyBuf, dk); err != nil {
			return err
		}
		l.Debugf("adding sequence; folder=%q sequence=%v %v", folder, f.Sequence, f.Name)

		if !f.IsDirectory() && !f.IsDeleted() && !f.IsInvalid() {
			for i, block := range f.Blocks {
				binary.BigEndian.PutUint32(blockBuf, uint32(i))
				keyBuf, err = db.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
				if err != nil {
					return err
				}
				if err := t.Put(keyBuf, blockBuf); err != nil {
					return err
				}
			}
		}

		if err := t.Checkpoint(); err != nil {
			return err
		}
	}

	return t.commit()
}

func (db *Lowlevel) dropFolder(folder []byte) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	// Remove all items related to the given folder from the device->file bucket
	k0, err := db.keyer.GenerateDeviceFileKey(nil, folder, nil, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k0.WithoutNameAndDevice()); err != nil {
		return err
	}

	// Remove all sequences related to the folder
	k1, err := db.keyer.GenerateSequenceKey(nil, folder, 0)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k1.WithoutSequence()); err != nil {
		return err
	}

	// Remove all items related to the given folder from the global bucket
	k2, err := db.keyer.GenerateGlobalVersionKey(nil, folder, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k2.WithoutName()); err != nil {
		return err
	}

	// Remove all needs related to the folder
	k3, err := db.keyer.GenerateNeedFileKey(nil, folder, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k3.WithoutName()); err != nil {
		return err
	}

	// Remove the blockmap of the folder
	k4, err := db.keyer.GenerateBlockMapKey(nil, folder, nil, nil)
	if err != nil {
		return err
	}
	if err := t.deleteKeyPrefix(k4.WithoutHashAndName()); err != nil {
		return err
	}

	return t.commit()
}

func (db *Lowlevel) dropDeviceFolder(device, folder []byte, meta *metadataTracker) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	key, err := db.keyer.GenerateDeviceFileKey(nil, folder, device, nil)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key)
	if err != nil {
		return err
	}
	var gk, keyBuf []byte
	for dbi.Next() {
		name := db.keyer.NameFromDeviceFileKey(dbi.Key())
		gk, err = db.keyer.GenerateGlobalVersionKey(gk, folder, name)
		if err != nil {
			return err
		}
		keyBuf, err = t.removeFromGlobal(gk, keyBuf, folder, device, name, meta)
		if err != nil {
			return err
		}
		if err := t.Delete(dbi.Key()); err != nil {
			return err
		}
		if err := t.Checkpoint(); err != nil {
			return err
		}
	}
	if err := dbi.Error(); err != nil {
		return err
	}
	dbi.Release()

	if bytes.Equal(device, protocol.LocalDeviceID[:]) {
		key, err := db.keyer.GenerateBlockMapKey(nil, folder, nil, nil)
		if err != nil {
			return err
		}
		if err := t.deleteKeyPrefix(key.WithoutHashAndName()); err != nil {
			return err
		}
	}
	return t.commit()
}

func (db *Lowlevel) checkGlobals(folder []byte, meta *metadataTracker) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	key, err := db.keyer.GenerateGlobalVersionKey(nil, folder, nil)
	if err != nil {
		return err
	}
	dbi, err := t.NewPrefixIterator(key.WithoutName())
	if err != nil {
		return err
	}
	defer dbi.Release()

	var dk []byte
	for dbi.Next() {
		vl, ok := unmarshalVersionList(dbi.Value())
		if !ok {
			continue
		}

		// Check the global version list for consistency. An issue in previous
		// versions of goleveldb could result in reordered writes so that
		// there are global entries pointing to no longer existing files. Here
		// we find those and clear them out.

		name := db.keyer.NameFromGlobalVersionKey(dbi.Key())
		var newVL VersionList
		for i, version := range vl.Versions {
			dk, err = db.keyer.GenerateDeviceFileKey(dk, folder, version.Device, name)
			if err != nil {
				return err
			}
			_, err := t.Get(dk)
			if backend.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			newVL.Versions = append(newVL.Versions, version)

			if i == 0 {
				if fi, ok, err := t.getFileByKey(dk); err != nil {
					return err
				} else if ok {
					meta.addFile(protocol.GlobalDeviceID, fi)
				}
			}
		}

		if len(newVL.Versions) != len(vl.Versions) {
			if err := t.Put(dbi.Key(), mustMarshal(&newVL)); err != nil {
				return err
			}
		}
	}
	if err := dbi.Error(); err != nil {
		return err
	}

	l.Debugf("db check completed for %q", folder)
	return t.commit()
}

func (db *Lowlevel) getIndexID(device, folder []byte) (protocol.IndexID, error) {
	key, err := db.keyer.GenerateIndexIDKey(nil, device, folder)
	if err != nil {
		return 0, err
	}
	cur, err := db.Get(key)
	if backend.IsNotFound(err) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	var id protocol.IndexID
	if err := id.Unmarshal(cur); err != nil {
		return 0, nil
	}

	return id, nil
}

func (db *Lowlevel) setIndexID(device, folder []byte, id protocol.IndexID) error {
	bs, _ := id.Marshal() // marshalling can't fail
	key, err := db.keyer.GenerateIndexIDKey(nil, device, folder)
	if err != nil {
		return err
	}
	return db.Put(key, bs)
}

func (db *Lowlevel) dropMtimes(folder []byte) error {
	key, err := db.keyer.GenerateMtimesKey(nil, folder)
	if err != nil {
		return err
	}
	return db.dropPrefix(key)
}

func (db *Lowlevel) dropFolderMeta(folder []byte) error {
	key, err := db.keyer.GenerateFolderMetaKey(nil, folder)
	if err != nil {
		return err
	}
	return db.dropPrefix(key)
}

func (db *Lowlevel) dropPrefix(prefix []byte) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	if err := t.deleteKeyPrefix(prefix); err != nil {
		return err
	}
	return t.commit()
}

func unmarshalTrunc(bs []byte, truncate bool) (FileIntf, error) {
	if truncate {
		var tf FileInfoTruncated
		err := tf.Unmarshal(bs)
		return tf, err
	}

	var tf protocol.FileInfo
	err := tf.Unmarshal(bs)
	return tf, err
}

func unmarshalVersionList(data []byte) (VersionList, bool) {
	var vl VersionList
	if err := vl.Unmarshal(data); err != nil {
		l.Debugln("unmarshal error:", err)
		return VersionList{}, false
	}
	if len(vl.Versions) == 0 {
		l.Debugln("empty version list")
		return VersionList{}, false
	}
	return vl, true
}

// unchanged checks if two files are the same and thus don't need to be updated.
// Local flags or the invalid bit might change without the version
// being bumped.
func unchanged(nf, ef FileIntf) bool {
	return ef.FileVersion().Equal(nf.FileVersion()) && ef.IsInvalid() == nf.IsInvalid() && ef.FileLocalFlags() == nf.FileLocalFlags()
}
