// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"bytes"
	"context"
	"hash"
	"hash/adler32"
	"io"
	"sync"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sha256"
)

var SHA256OfNothing = []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}

type Counter interface {
	Update(bytes int64)
}

// Blocks returns the blockwise hash of the reader.
func Blocks(ctx context.Context, r io.Reader, blocksize int, sizehint int64, counter Counter, useWeakHashes bool) ([]protocol.BlockInfo, error) {
	if counter == nil {
		counter = &noopCounter{}
	}

	const hashLength = sha256.Size
	multiHf := newMultiHash(useWeakHashes)

	var blocks []protocol.BlockInfo
	var hashes, thisHash []byte

	if sizehint >= 0 {
		// Allocate contiguous blocks for the BlockInfo structures and their
		// hashes once and for all, and stick to the specified size.
		r = io.LimitReader(r, sizehint)
		numBlocks := int(sizehint / int64(blocksize))
		blocks = make([]protocol.BlockInfo, 0, numBlocks)
		hashes = make([]byte, 0, hashLength*numBlocks)
	}

	// A 32k buffer is used for copying into the hash function.
	buf := make([]byte, 32<<10)

	var offset int64
	lr := io.LimitReader(r, int64(blocksize)).(*io.LimitedReader)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		lr.N = int64(blocksize)
		n, err := io.CopyBuffer(multiHf, lr, buf)
		if err != nil {
			return nil, err
		}

		if n == 0 {
			break
		}

		counter.Update(n)

		// Carve out a hash-sized chunk of "hashes" to store the hash for this
		// block.
		hashes = multiHf.strongSum(hashes)
		thisHash, hashes = hashes[:hashLength], hashes[hashLength:]

		b := protocol.BlockInfo{
			Size:     int32(n),
			Offset:   offset,
			Hash:     thisHash,
			WeakHash: multiHf.weakSum(),
		}

		blocks = append(blocks, b)
		offset += n

		multiHf.reset()
	}

	if len(blocks) == 0 {
		// Empty file
		blocks = append(blocks, protocol.BlockInfo{
			Offset: 0,
			Size:   0,
			Hash:   SHA256OfNothing,
		})
	}

	multiHashPool.Put(multiHf)
	return blocks, nil
}

type multiHash struct {
	strong hash.Hash
	weak   hash.Hash32
}

var multiHashPool sync.Pool

func newMultiHash(useWeakHashes bool) *multiHash {
	h, ok := multiHashPool.Get().(*multiHash)
	if !ok {
		h = &multiHash{strong: sha256.New()}
	}

	if !useWeakHashes {
		h.weak = nil
	} else if h.weak == nil {
		h.weak = adler32.New()
	}

	h.reset()
	return h
}

func (h *multiHash) reset() {
	h.strong.Reset()
	if h.weak != nil {
		h.weak.Reset()
	}
}

func (h *multiHash) strongSum(d []byte) []byte {
	return h.strong.Sum(d)
}

func (h *multiHash) weakSum() uint32 {
	if h.weak == nil {
		return 0
	}
	return h.weak.Sum32()
}

func (h *multiHash) Write(p []byte) (n int, err error) {
	h.strong.Write(p)
	if h.weak != nil {
		h.weak.Write(p)
	}
	return len(p), nil
}

// Validate quickly validates buf against the cryptohash hash (if len(hash)>0)
// and the 32-bit hash weakHash (if not zero). It is satisfied if either hash
// matches, or neither is given.
func Validate(buf, hash []byte, weakHash uint32) bool {
	if weakHash != 0 {
		return adler32.Checksum(buf) == weakHash
	}

	if len(hash) > 0 {
		hbuf := sha256.Sum256(buf)
		return bytes.Equal(hbuf[:], hash)
	}

	return true
}

type noopCounter struct{}

func (c *noopCounter) Update(bytes int64) {}
