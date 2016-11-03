// Copyright 2016 The Internal Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package file provides an os.File-like interface of a memory mapped file.
package file

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cznic/fileutil"
	"github.com/cznic/internal/buffer"
	"github.com/cznic/mathutil"
	"github.com/edsrzf/mmap-go"
)

const copyBufSize = 1 << 20 // 1 MB.

var (
	_ Interface = (*mem)(nil)
	_ Interface = (*file)(nil)

	_ os.FileInfo = stat{}

	sysPage = os.Getpagesize()
)

// Interface is a os.File-like entity.
type Interface interface {
	io.ReaderAt
	io.ReaderFrom
	io.WriterAt
	io.WriterTo

	Close() error
	Stat() (os.FileInfo, error)
	Sync() error
	Truncate(int64) error
}

// Open returns a new Interface backed by f, or an error, if any.
func Open(f *os.File) (Interface, error) { return newFile(f, 1<<30, 20) }

// OpenMem returns a new Interface, or an error, if any. The Interface content
// is volatile, it's backed only by process' memory.
func OpenMem(name string) (Interface, error) { return newMem(name, 18), nil }

type memMap map[int64]*[]byte

type mem struct {
	m       memMap
	modTime time.Time
	name    string
	pgBits  uint
	pgMask  int
	pgSize  int
	size    int64
}

func newMem(name string, pgBits uint) *mem {
	pgSize := 1 << pgBits
	return &mem{
		m:       memMap{},
		modTime: time.Now(),
		name:    name,
		pgBits:  pgBits,
		pgMask:  pgSize - 1,
		pgSize:  pgSize,
	}
}

func (f *mem) IsDir() bool                               { return false }
func (f *mem) Mode() os.FileMode                         { return os.ModeTemporary + 0600 }
func (f *mem) ModTime() time.Time                        { return f.modTime }
func (f *mem) Name() string                              { return f.name }
func (f *mem) ReadFrom(r io.Reader) (n int64, err error) { return readFrom(f, r) }
func (f *mem) Size() (n int64)                           { return f.size }
func (f *mem) Stat() (os.FileInfo, error)                { return f, nil }
func (f *mem) Sync() error                               { return nil }
func (f *mem) Sys() interface{}                          { return nil }
func (f *mem) WriteTo(w io.Writer) (n int64, err error)  { return writeTo(f, w) }

func (f *mem) Close() error {
	f.Truncate(0)
	f.m = nil
	return nil
}

func (f *mem) ReadAt(b []byte, off int64) (n int, err error) {
	avail := f.size - off
	pi := off >> f.pgBits
	po := int(off) & f.pgMask
	rem := len(b)
	if int64(rem) >= avail {
		rem = int(avail)
		err = io.EOF
	}
	var zeroPage *[]byte
	for rem != 0 && avail > 0 {
		pg := f.m[pi]
		if pg == nil {
			if zeroPage == nil {
				zeroPage = buffer.CGet(f.pgSize)
				defer buffer.Put(zeroPage)
			}
			pg = zeroPage
		}
		nc := copy(b[:mathutil.Min(rem, f.pgSize)], (*pg)[po:])
		pi++
		po = 0
		rem -= nc
		n += nc
		b = b[nc:]
	}
	return n, err
}

func (f *mem) Truncate(size int64) (err error) {
	if size < 0 {
		return fmt.Errorf("invalid truncate size: %d", size)
	}

	first := size >> f.pgBits
	if size&int64(f.pgMask) != 0 {
		first++
	}
	last := f.size >> f.pgBits
	if f.size&int64(f.pgMask) != 0 {
		last++
	}
	for ; first <= last; first++ {
		if p := f.m[first]; p != nil {
			buffer.Put(p)
		}
		delete(f.m, first)
	}

	f.size = size
	return nil
}

func (f *mem) WriteAt(b []byte, off int64) (n int, err error) {
	pi := off >> f.pgBits
	po := int(off) & f.pgMask
	n = len(b)
	rem := n
	var nc int
	for rem != 0 {
		pg := f.m[pi]
		if pg == nil {
			pg = buffer.CGet(f.pgSize)
			f.m[pi] = pg
		}
		nc = copy((*pg)[po:], b)
		pi++
		po = 0
		rem -= nc
		b = b[nc:]
	}
	f.size = mathutil.MaxInt64(f.size, off+int64(n))
	return n, nil
}

type stat struct {
	os.FileInfo
	size int64
}

func (s stat) Size() int64 { return s.size }

type fileMap map[int64]mmap.MMap

type file struct {
	f        *os.File
	m        fileMap
	maxPages int
	pgBits   uint
	pgMask   int
	pgSize   int
	size     int64
	fsize    int64
}

func newFile(f *os.File, maxSize int64, pgBits uint) (*file, error) {
	if maxSize < 0 {
		panic("internal error")
	}

	pgSize := 1 << pgBits
	switch {
	case sysPage > pgSize:
		pgBits = uint(mathutil.Log2Uint64(uint64(sysPage)))
	default:
		pgBits = uint(mathutil.Log2Uint64(uint64(pgSize / sysPage * sysPage)))
	}
	pgSize = 1 << pgBits
	fi := &file{
		f: f,
		m: fileMap{},
		maxPages: int(mathutil.MinInt64(
			1024,
			mathutil.MaxInt64(maxSize/int64(pgSize), 1)),
		),
		pgBits: pgBits,
		pgMask: pgSize - 1,
		pgSize: pgSize,
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if err = fi.Truncate(info.Size()); err != nil {
		return nil, err
	}

	return fi, nil
}

func (f *file) ReadFrom(r io.Reader) (n int64, err error) { return readFrom(f, r) }
func (f *file) Sync() (err error)                         { return f.f.Sync() }
func (f *file) WriteTo(w io.Writer) (n int64, err error)  { return writeTo(f, w) }

func (f *file) Close() (err error) {
	for _, p := range f.m {
		if err = p.Unmap(); err != nil {
			return err
		}
	}

	if err = f.f.Truncate(f.size); err != nil {
		return err
	}

	if err = f.f.Sync(); err != nil {
		return err
	}

	if err = f.f.Close(); err != nil {
		return err
	}

	f.m = nil
	f.f = nil
	return nil
}

func (f *file) page(index int64) (mmap.MMap, error) {
	if len(f.m) == f.maxPages {
		for i, p := range f.m {
			if err := p.Unmap(); err != nil {
				return nil, err
			}

			delete(f.m, i)
			break
		}
	}

	off := index << f.pgBits
	fsize := off + int64(f.pgSize)
	if fsize > f.fsize {
		if err := f.f.Truncate(fsize); err != nil {
			return nil, err
		}

		f.fsize = fsize
	}
	p, err := mmap.MapRegion(f.f, f.pgSize, mmap.RDWR, 0, off)
	if err != nil {
		return nil, err
	}

	f.m[index] = p
	return p, nil
}

func (f *file) ReadAt(b []byte, off int64) (n int, err error) {
	avail := f.size - off
	pi := off >> f.pgBits
	po := int(off) & f.pgMask
	rem := len(b)
	if int64(rem) >= avail {
		rem = int(avail)
		err = io.EOF
	}
	for rem != 0 && avail > 0 {
		pg := f.m[pi]
		if pg == nil {
			if pg, err = f.page(pi); err != nil {
				return n, err
			}
		}
		nc := copy(b[:mathutil.Min(rem, f.pgSize)], pg[po:])
		pi++
		po = 0
		rem -= nc
		n += nc
		b = b[nc:]
	}
	return n, err
}

func (f *file) Stat() (os.FileInfo, error) {
	fi, err := f.f.Stat()
	if err != nil {
		return nil, err
	}

	return stat{fi, f.size}, nil
}

func (f *file) Truncate(size int64) (err error) {
	if size < 0 {
		return fmt.Errorf("invalid truncate size: %d", size)
	}

	first := size >> f.pgBits
	if size&int64(f.pgMask) != 0 {
		first++
	}
	last := f.size >> f.pgBits
	if f.size&int64(f.pgMask) != 0 {
		last++
	}
	for ; first <= last; first++ {
		if p := f.m[first]; p != nil {
			if err := p.Unmap(); err != nil {
				return err
			}
		}

		delete(f.m, first)
	}

	f.size = size
	fsize := (size + int64(f.pgSize) - 1) &^ int64(f.pgMask)
	if fsize != f.fsize {
		if err := f.f.Truncate(fsize); err != nil {
			return err
		}

	}
	f.fsize = fsize
	return nil
}

func (f *file) WriteAt(b []byte, off int64) (n int, err error) {
	pi := off >> f.pgBits
	po := int(off) & f.pgMask
	n = len(b)
	rem := n
	var nc int
	for rem != 0 {
		pg := f.m[pi]
		if pg == nil {
			pg, err = f.page(pi)
			if err != nil {
				return n, err
			}
		}
		nc = copy(pg[po:], b)
		pi++
		po = 0
		rem -= nc
		b = b[nc:]
	}
	f.size = mathutil.MaxInt64(f.size, off+int64(n))
	return n, nil
}

// ----------------------------------------------------------------------------

func readFrom(f Interface, r io.Reader) (n int64, err error) {
	f.Truncate(0)
	p := buffer.Get(copyBufSize)
	b := *p
	defer buffer.Put(p)

	var off int64
	var werr error
	for {
		rn, rerr := r.Read(b)
		if rn != 0 {
			_, werr = f.WriteAt(b[:rn], off)
			n += int64(rn)
			off += int64(rn)
		}
		if rerr != nil {
			if !fileutil.IsEOF(rerr) {
				err = rerr
			}
			break
		}

		if werr != nil {
			err = werr
			break
		}
	}
	return n, err
}

func writeTo(f Interface, w io.Writer) (n int64, err error) {
	p := buffer.Get(copyBufSize)
	b := *p
	defer buffer.Put(p)

	var off int64
	var werr error
	for {
		rn, rerr := f.ReadAt(b, off)
		if rn != 0 {
			_, werr = w.Write(b[:rn])
			n += int64(rn)
			off += int64(rn)
		}
		if rerr != nil {
			if !fileutil.IsEOF(rerr) {
				err = rerr
			}
			break
		}

		if werr != nil {
			err = werr
			break
		}
	}
	return n, err
}
