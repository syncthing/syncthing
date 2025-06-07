// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package olddb

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/db/olddb/backend"
	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/internal/gen/dbproto"
	"github.com/syncthing/syncthing/lib/protocol"
)

// A readOnlyTransaction represents a database snapshot.
type readOnlyTransaction struct {
	backend.ReadTransaction
	keyer keyer
}

func (db *deprecatedLowlevel) newReadOnlyTransaction() (readOnlyTransaction, error) {
	tran, err := db.NewReadTransaction()
	if err != nil {
		return readOnlyTransaction{}, err
	}
	return db.readOnlyTransactionFromBackendTransaction(tran), nil
}

func (db *deprecatedLowlevel) readOnlyTransactionFromBackendTransaction(tran backend.ReadTransaction) readOnlyTransaction {
	return readOnlyTransaction{
		ReadTransaction: tran,
		keyer:           db.keyer,
	}
}

func (t readOnlyTransaction) close() {
	t.Release()
}

func (t readOnlyTransaction) getFileByKey(key []byte) (protocol.FileInfo, bool, error) {
	f, ok, err := t.getFileTrunc(key, false)
	if err != nil || !ok {
		return protocol.FileInfo{}, false, err
	}
	return f, true, nil
}

func (t readOnlyTransaction) getFileTrunc(key []byte, trunc bool) (protocol.FileInfo, bool, error) {
	bs, err := t.Get(key)
	if backend.IsNotFound(err) {
		return protocol.FileInfo{}, false, nil
	}
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	f, err := t.unmarshalTrunc(bs, trunc)
	if backend.IsNotFound(err) {
		return protocol.FileInfo{}, false, nil
	}
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	return f, true, nil
}

func (t readOnlyTransaction) unmarshalTrunc(bs []byte, trunc bool) (protocol.FileInfo, error) {
	if trunc {
		var bfi dbproto.FileInfoTruncated
		err := proto.Unmarshal(bs, &bfi)
		if err != nil {
			return protocol.FileInfo{}, err
		}
		if err := t.fillTruncated(&bfi); err != nil {
			return protocol.FileInfo{}, err
		}
		return protocol.FileInfoFromDBTruncated(&bfi), nil
	}

	var bfi bep.FileInfo
	err := proto.Unmarshal(bs, &bfi)
	if err != nil {
		return protocol.FileInfo{}, err
	}
	if err := t.fillFileInfo(&bfi); err != nil {
		return protocol.FileInfo{}, err
	}
	return protocol.FileInfoFromDB(&bfi), nil
}

type blocksIndirectionError struct {
	err error
}

func (e *blocksIndirectionError) Error() string {
	return fmt.Sprintf("filling Blocks: %v", e.err)
}

func (e *blocksIndirectionError) Unwrap() error {
	return e.err
}

// fillFileInfo follows the (possible) indirection of blocks and version
// vector and fills it out.
func (t readOnlyTransaction) fillFileInfo(fi *bep.FileInfo) error {
	var key []byte

	if len(fi.Blocks) == 0 && len(fi.BlocksHash) != 0 {
		// The blocks list is indirected and we need to load it.
		key = t.keyer.GenerateBlockListKey(key, fi.BlocksHash)
		bs, err := t.Get(key)
		if err != nil {
			return &blocksIndirectionError{err}
		}
		var bl dbproto.BlockList
		if err := proto.Unmarshal(bs, &bl); err != nil {
			return err
		}
		fi.Blocks = bl.Blocks
	}

	if len(fi.VersionHash) != 0 {
		key = t.keyer.GenerateVersionKey(key, fi.VersionHash)
		bs, err := t.Get(key)
		if err != nil {
			return fmt.Errorf("filling Version: %w", err)
		}
		var v bep.Vector
		if err := proto.Unmarshal(bs, &v); err != nil {
			return err
		}
		fi.Version = &v
	}

	return nil
}

// fillTruncated follows the (possible) indirection of version vector and
// fills it.
func (t readOnlyTransaction) fillTruncated(fi *dbproto.FileInfoTruncated) error {
	var key []byte

	if len(fi.VersionHash) == 0 {
		return nil
	}

	key = t.keyer.GenerateVersionKey(key, fi.VersionHash)
	bs, err := t.Get(key)
	if err != nil {
		return err
	}
	var v bep.Vector
	if err := proto.Unmarshal(bs, &v); err != nil {
		return err
	}
	fi.Version = &v
	return nil
}

func (t *readOnlyTransaction) withHaveSequence(folder []byte, startSeq int64, fn Iterator) error {
	first, err := t.keyer.GenerateSequenceKey(nil, folder, startSeq)
	if err != nil {
		return err
	}
	last, err := t.keyer.GenerateSequenceKey(nil, folder, maxInt64)
	if err != nil {
		return err
	}
	dbi, err := t.NewRangeIterator(first, last)
	if err != nil {
		return err
	}
	defer dbi.Release()

	for dbi.Next() {
		f, ok, err := t.getFileByKey(dbi.Value())
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		if !fn(f) {
			return nil
		}
	}
	return dbi.Error()
}
