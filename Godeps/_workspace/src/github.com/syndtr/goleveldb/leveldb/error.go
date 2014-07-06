// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"errors"

	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	ErrNotFound         = util.ErrNotFound
	ErrSnapshotReleased = errors.New("leveldb: snapshot released")
	ErrIterReleased     = errors.New("leveldb: iterator released")
	ErrClosed           = errors.New("leveldb: closed")
)

type CorruptionType int

const (
	CorruptedManifest CorruptionType = iota
	MissingFiles
)

// ErrCorrupted is the type that wraps errors that indicate corruption in
// the database.
type ErrCorrupted struct {
	Type CorruptionType
	Err  error
}

func (e ErrCorrupted) Error() string {
	return e.Err.Error()
}
