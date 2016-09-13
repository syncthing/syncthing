// Copyright 2014 The lldb Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// A basic os.File backed Filer.

package lldb

import (
	"os"

	"github.com/cznic/internal/file"
)

var _ Filer = &SimpleFileFiler{}

// SimpleFileFiler is an os.File backed Filer intended for use where structural
// consistency can be reached by other means (SimpleFileFiler is for example
// wrapped in eg. an RollbackFiler or ACIDFiler0) or where persistence is not
// required (temporary/working data sets).
//
// SimpleFileFiler is the most simple os.File backed Filer implementation as it
// does not really implement BeginUpdate and EndUpdate/Rollback in any way
// which would protect the structural integrity of data. If misused e.g. as a
// real database storage w/o other measures, it can easily cause data loss
// when, for example, a power outage occurs or the updating process terminates
// abruptly.
type SimpleFileFiler struct {
	fi   file.Interface
	name string
	nest int
}

// NewSimpleFileFiler returns a new SimpleFileFiler.
func NewSimpleFileFiler(f *os.File) *SimpleFileFiler {
	fi, err := file.Open(f)
	if err != nil {
		return nil
	}

	sf := &SimpleFileFiler{fi: fi, name: f.Name()}
	return sf
}

// BeginUpdate implements Filer.
func (f *SimpleFileFiler) BeginUpdate() error {
	f.nest++
	return nil
}

// Close implements Filer.
func (f *SimpleFileFiler) Close() (err error) {
	if f.nest != 0 {
		return &ErrPERM{(f.Name() + ":Close")}
	}

	return f.fi.Close()
}

// EndUpdate implements Filer.
func (f *SimpleFileFiler) EndUpdate() (err error) {
	if f.nest == 0 {
		return &ErrPERM{(f.Name() + ":EndUpdate")}
	}

	f.nest--
	return
}

// Name implements Filer.
func (f *SimpleFileFiler) Name() string { return f.name }

// PunchHole implements Filer.
func (f *SimpleFileFiler) PunchHole(off, size int64) (err error) { return nil }

// ReadAt implements Filer.
func (f *SimpleFileFiler) ReadAt(b []byte, off int64) (n int, err error) { return f.fi.ReadAt(b, off) }

// Rollback implements Filer.
func (f *SimpleFileFiler) Rollback() (err error) { return nil }

// Size implements Filer.
func (f *SimpleFileFiler) Size() (int64, error) {
	info, err := f.fi.Stat()
	if err != nil {
		return 0, err
	}

	return info.Size(), nil
}

// Sync implements Filer.
func (f *SimpleFileFiler) Sync() error { return f.fi.Sync() }

// Truncate implements Filer.
func (f *SimpleFileFiler) Truncate(size int64) (err error) { return f.fi.Truncate(size) }

// WriteAt implements Filer.
func (f *SimpleFileFiler) WriteAt(b []byte, off int64) (n int, err error) { return f.fi.WriteAt(b, off) }
