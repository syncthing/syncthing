// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package errors provides common error types used throughout leveldb.
package errors

import (
	"errors"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	ErrNotFound    = New("leveldb: not found")
	ErrReleased    = util.ErrReleased
	ErrHasReleaser = util.ErrHasReleaser
)

// New returns an error that formats as the given text.
func New(text string) error {
	return errors.New(text)
}

// ErrCorrupted is the type that wraps errors that indicate corruption in
// the database.
type ErrCorrupted struct {
	File *storage.FileInfo
	Err  error
}

func (e *ErrCorrupted) Error() string {
	if e.File != nil {
		return fmt.Sprintf("%v [file=%v]", e.Err, e.File)
	} else {
		return e.Err.Error()
	}
}

// NewErrCorrupted creates new ErrCorrupted error.
func NewErrCorrupted(f storage.File, err error) error {
	return &ErrCorrupted{storage.NewFileInfo(f), err}
}

// IsCorrupted returns a boolean indicating whether the error is indicating
// a corruption.
func IsCorrupted(err error) bool {
	switch err.(type) {
	case *ErrCorrupted:
		return true
	}
	return false
}

// ErrMissingFiles is the type that indicating a corruption due to missing
// files.
type ErrMissingFiles struct {
	Files []*storage.FileInfo
}

func (e *ErrMissingFiles) Error() string { return "file missing" }

// SetFile sets 'file info' of the given error with the given file.
// Currently only ErrCorrupted is supported, otherwise will do nothing.
func SetFile(err error, f storage.File) error {
	switch x := err.(type) {
	case *ErrCorrupted:
		x.File = storage.NewFileInfo(f)
		return x
	}
	return err
}
