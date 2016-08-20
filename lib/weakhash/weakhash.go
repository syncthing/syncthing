// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package weakhash

import (
	"hash"
	"io"
)

const (
	Size = 4
)

func New(size int) hash.Hash32 {
	return &digest{
		buf:  make([]byte, size),
		size: size,
	}
}

// Find finds all the blocks of the given size within io.Reader that matches
// the hashes provided in the search, and returns a map, mapping each hash being
// searched for, to a list of offsets within io.Reader that would produce the
// same weak hash.
func Find(r io.Reader, hashesToFind []uint32, size int) (map[uint32][]int64, error) {
	if r == nil {
		return nil, nil
	}
	hf := New(size)

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

	hash := hf.Sum32()
	if _, ok := offsets[hash]; ok {
		offsets[hash] = []int64{0}
	}

	buf := make([]byte, 1)

	var i int64 = 1
	for {
		_, err := r.Read(buf)
		if err == io.EOF {
			break
		} else if err != nil {
			return offsets, err
		}
		hf.Write(buf)
		hash = hf.Sum32()
		if existing, ok := offsets[hash]; ok {
			offsets[hash] = append(existing, i)
		}
		i++
	}
	return offsets, nil
}

// Using this: http://tutorials.jenkov.com/rsync/checksums.html
// Alternative is adler32 http://blog.liw.fi/posts/rsync-in-python/#comment-fee8d5e07794fdba3fe2d76aa2706a13
type digest struct {
	buf  []byte
	size int
	a    uint16
	b    uint16
	j    int
}

func (d *digest) Write(data []byte) (int, error) {
	for _, c := range data {
		d.a = d.a - uint16(d.buf[d.j]) + uint16(c)
		d.b = d.b - uint16(d.size)*uint16(d.buf[d.j]) + d.a
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
