// Copyright 2014 The lldb Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// A memory-only implementation of Filer.

package lldb

import (
	"fmt"
	"io"

	"github.com/cznic/internal/file"
)

var _ Filer = &MemFiler{}

// MemFiler is a memory backed Filer. It implements BeginUpdate, EndUpdate and
// Rollback as no-ops. MemFiler is not automatically persistent, but it has
// ReadFrom and WriteTo methods.
type MemFiler struct {
	fi   file.Interface
	nest int
}

// NewMemFiler returns a new MemFiler.
func NewMemFiler() *MemFiler {
	fi, err := file.OpenMem("")
	if err != nil {
		return nil
	}

	return &MemFiler{fi: fi}
}

// BeginUpdate implements Filer.
func (f *MemFiler) BeginUpdate() error {
	f.nest++
	return nil
}

// Close implements Filer.
func (f *MemFiler) Close() (err error) {
	if f.nest != 0 {
		return &ErrPERM{(f.Name() + ":Close")}
	}

	return f.fi.Close()
}

// EndUpdate implements Filer.
func (f *MemFiler) EndUpdate() (err error) {
	if f.nest == 0 {
		return &ErrPERM{(f.Name() + ": EndUpdate")}
	}

	f.nest--
	return
}

// Name implements Filer.
func (f *MemFiler) Name() string { return fmt.Sprintf("%p.memfiler", f) }

// PunchHole implements Filer.
func (f *MemFiler) PunchHole(off, size int64) (err error) { return nil }

// ReadAt implements Filer.
func (f *MemFiler) ReadAt(b []byte, off int64) (n int, err error) { return f.fi.ReadAt(b, off) }

// ReadFrom is a helper to populate MemFiler's content from r.  'n' reports the
// number of bytes read from 'r'.
func (f *MemFiler) ReadFrom(r io.Reader) (n int64, err error) { return f.fi.ReadFrom(r) }

// Rollback implements Filer.
func (f *MemFiler) Rollback() (err error) { return nil }

// Size implements Filer.
func (f *MemFiler) Size() (int64, error) {
	info, err := f.fi.Stat()
	if err != nil {
		return 0, err
	}

	return info.Size(), nil
}

// Sync implements Filer.
func (f *MemFiler) Sync() error { return nil }

// Truncate implements Filer.
func (f *MemFiler) Truncate(size int64) (err error) { return f.fi.Truncate(size) }

// WriteAt implements Filer.
func (f *MemFiler) WriteAt(b []byte, off int64) (n int, err error) { return f.fi.WriteAt(b, off) }

// WriteTo is a helper to copy/persist MemFiler's content to w.  If w is also
// an io.WriterAt then WriteTo may attempt to _not_ write any big, for some
// value of big, runs of zeros, i.e. it will attempt to punch holes, where
// possible, in `w` if that happens to be a freshly created or to zero length
// truncated OS file.  'n' reports the number of bytes written to 'w'.
func (f *MemFiler) WriteTo(w io.Writer) (n int64, err error) { return f.fi.WriteTo(w) }
