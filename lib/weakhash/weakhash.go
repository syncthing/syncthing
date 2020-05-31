// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package weakhash

import (
	"bufio"
	"context"
	"io"

	"github.com/chmduquesne/rollinghash/adler32"
)

// A Finder scans through an io.ReaderAt, looking for blocks whose Adler-32
// hash is in a given set.
type Finder struct {
	buf    []byte
	err    error
	hashes map[uint32]struct{}
	offset int64

	hf *adler32.Adler32

	r  io.ReaderAt
	br *bufio.Reader

	// Last matching hash value.
	match uint32
}

type Reader interface {
	io.ReaderAt
	io.Reader
}

// NewFinder returns a Finder that initially has no hashes.
// Call Add before Next to add hashes.
//
// The buffer buf will be filled for each match found. Its length is taken
// to be the block size.
func NewFinder(r Reader, buf []byte) *Finder {
	f := &Finder{
		buf:    buf,
		hashes: make(map[uint32]struct{}),
		hf:     adler32.New(),
		r:      r,
		br:     bufio.NewReader(r),
	}

	return f
}

// Add adds the hash h to f.
func (f *Finder) Add(h uint32) { f.hashes[h] = struct{}{} }

// Err returns the last error encountered by Next, if any.
// EOF is not considered an error.
func (f *Finder) Err() error {
	switch f.err {
	case io.EOF, io.ErrUnexpectedEOF:
		return nil
	default:
		return f.err
	}
}

// Match returns the hash and offset of the last match found by Next.
func (f *Finder) Match() (h uint32, offset int64) {
	return f.match, f.offset - int64(len(f.buf))
}

// Next returns true if it can find a match for any of f's hashes,
// false if it encounters either an error or EOF.
//
// When Next has returned true, the contents of the block found are in the
// buffer handed to NewFinder.
func (f *Finder) Next(ctx context.Context) bool {
	if len(f.hashes) == 0 {
		return false
	}

	blockSize := int64(len(f.buf))

	if f.offset < blockSize {
		// Initialize by reading the first blockSize bytes. ReadAt must
		// return an error when it reads less than len(f.buf) bytes.
		_, err := io.ReadFull(f.br, f.buf)
		if err != nil {
			f.err = err
			return false
		}

		f.hf.Write(f.buf)
		f.offset = blockSize

		h := f.hf.Sum32()
		if _, ok := f.hashes[h]; ok {
			f.match = h
			return true
		}
	}

	for {
		bt, err := f.br.ReadByte()
		if err != nil {
			f.err = err
			return false
		}
		f.hf.Roll(bt)
		f.offset++

		h := f.hf.Sum32()
		if _, ok := f.hashes[h]; ok {
			// We have to read the block again here, because the rollinghash
			// library does not provide access to its buffers. This is wasteful
			// because the block is already in memory somewhere, but at least
			// it will likely still be in the disk cache.
			_, f.err = f.r.ReadAt(f.buf, f.offset-blockSize)
			if f.err != nil {
				return false
			}
			f.match = h
			return true
		}

		// Periodically check whether the context has expired.
		// The 16kB period is entirely arbitrary.
		if f.offset%16384 == 0 {
			select {
			case <-ctx.Done():
				f.err = ctx.Err()
				return false
			default:
			}
		}
	}
}

// Remove tells f to not longer look for h.
func (f *Finder) Remove(h uint32) {
	delete(f.hashes, h)
}
