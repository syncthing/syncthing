// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package weakhash

import (
	"bufio"
	"hash"
	"io"
	"os"
)

const (
	Size = 4
)

func NewHash(size int) hash.Hash32 {
	return &digest{
		buf:  make([]byte, size),
		size: size,
	}
}

// Find finds all the blocks of the given size within io.Reader that matches
// the hashes provided, and returns a hash -> slice of offsets within reader
// map, that produces the same weak hash.
func Find(ir io.Reader, hashesToFind []uint32, size int) (map[uint32][]int64, error) {
	if ir == nil {
		return nil, nil
	}

	r := bufio.NewReader(ir)
	hf := NewHash(size)

	n, err := io.CopyN(hf, r, int64(size))
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if n != int64(size) {
		return nil, io.ErrShortBuffer
	}

	offsets := make(map[uint32][]int64)
	for _, hashToFind := range hashesToFind {
		offsets[hashToFind] = nil
	}

	var i int64
	var hash uint32
	for {
		hash = hf.Sum32()
		if existing, ok := offsets[hash]; ok {
			offsets[hash] = append(existing, i)
		}
		i++

		bt, err := r.ReadByte()
		if err == io.EOF {
			break
		} else if err != nil {
			return offsets, err
		}
		hf.Write([]byte{bt})
	}
	return offsets, nil
}

// Using this: http://tutorials.jenkov.com/rsync/checksums.html
// Example implementations: https://gist.github.com/csabahenk/1096262/revisions
// Alternative that could be used is adler32 http://blog.liw.fi/posts/rsync-in-python/#comment-fee8d5e07794fdba3fe2d76aa2706a13
type digest struct {
	buf  []byte
	size int
	a    uint16
	b    uint16
	j    int
}

func (d *digest) Write(data []byte) (int, error) {
	for _, c := range data {
		// TODO: Use this in Go 1.6
		// d.a = d.a - uint16(d.buf[d.j]) + uint16(c)
		// d.b = d.b - uint16(d.size)*uint16(d.buf[d.j]) + d.a
		d.a -= uint16(d.buf[d.j])
		d.a += uint16(c)
		d.b -= uint16(d.size) * uint16(d.buf[d.j])
		d.b += d.a

		d.buf[d.j] = c
		d.j = (d.j + 1) % d.size
	}
	return len(data), nil
}

func (d *digest) Reset() {
	for i := range d.buf {
		d.buf[i] = 0x0
	}
	d.a = 0
	d.b = 0
	d.j = 0
}

func (d *digest) Sum(b []byte) []byte {
	r := d.Sum32()
	return append(b, byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
}

func (d *digest) Sum32() uint32 { return uint32(d.a) | (uint32(d.b) << 16) }
func (digest) Size() int        { return Size }
func (digest) BlockSize() int   { return 1 }

func NewFinder(path string, size int, hashesToFind []uint32) (*Finder, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	offsets, err := Find(file, hashesToFind, size)
	if err != nil {
		file.Close()
		return nil, err
	}

	return &Finder{
		file:    file,
		size:    size,
		offsets: offsets,
	}, nil
}

type Finder struct {
	file    *os.File
	size    int
	offsets map[uint32][]int64
}

// Iterate iterates all available blocks that matches the provided hash, reads
// them into buf, and calls the iterator function. The iterator function should
// return wether it wishes to continue interating.
func (h *Finder) Iterate(hash uint32, buf []byte, iterFunc func(int64) bool) (bool, error) {
	if h == nil || hash == 0 || len(buf) != h.size {
		return false, nil
	}

	for _, offset := range h.offsets[hash] {
		_, err := h.file.ReadAt(buf, offset)
		if err != nil {
			return false, err
		}
		if !iterFunc(offset) {
			return true, nil
		}
	}
	return false, nil
}

// Close releases any resource associated with the finder
func (h *Finder) Close() {
	if h != nil {
		h.file.Close()
	}
}
