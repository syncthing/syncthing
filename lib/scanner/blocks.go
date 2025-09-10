// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"hash"
	"io"
	"sync"

	"github.com/syncthing/syncthing/lib/protocol"
)

var SHA256OfNothing = []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}

type Counter interface {
	Update(bytes int64)
}

const bufSize = 32 << 10 // 32k

var bufPool = sync.Pool{
	New: func() any {
		return new([bufSize]byte) // 32k buffer
	},
}

const hashLength = sha256.Size

var hashPool = sync.Pool{
	New: func() any {
		return sha256.New()
	},
}

// Blocks returns the blockwise hash of the reader.
func Blocks(ctx context.Context, r io.Reader, blocksize int, sizehint int64, counter Counter) ([]protocol.BlockInfo, error) {
	if counter == nil {
		counter = &noopCounter{}
	}

	var blocks []protocol.BlockInfo
	var hashes, thisHash []byte

	if sizehint >= 0 {
		// Allocate contiguous blocks for the BlockInfo structures and their
		// hashes once and for all, and stick to the specified size.
		r = io.LimitReader(r, sizehint)
		numBlocks := sizehint / int64(blocksize)
		remainder := sizehint % int64(blocksize)
		if remainder != 0 {
			numBlocks++
		}
		blocks = make([]protocol.BlockInfo, 0, numBlocks)
		hashes = make([]byte, 0, hashLength*numBlocks)
	}

	hf := hashPool.Get().(hash.Hash) //nolint:forcetypeassert
	// A 32k buffer is used for copying into the hash function.
	buf := bufPool.Get().(*[bufSize]byte)[:] //nolint:forcetypeassert
	defer func() {
		bufPool.Put((*[bufSize]byte)(buf))
		hf.Reset()
		hashPool.Put(hf)
	}()

	var offset int64
	lr := io.LimitReader(r, int64(blocksize)).(*io.LimitedReader)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		lr.N = int64(blocksize)
		n, err := io.CopyBuffer(hf, lr, buf)
		if err != nil {
			return nil, err
		}

		if n == 0 {
			break
		}

		counter.Update(n)

		// Carve out a hash-sized chunk of "hashes" to store the hash for this
		// block.
		hashes = hf.Sum(hashes)
		thisHash, hashes = hashes[:hashLength], hashes[hashLength:]

		b := protocol.BlockInfo{
			Size:   int(n),
			Offset: offset,
			Hash:   thisHash,
		}

		blocks = append(blocks, b)
		offset += n

		hf.Reset()
	}

	if len(blocks) == 0 {
		// Empty file
		blocks = append(blocks, protocol.BlockInfo{
			Offset: 0,
			Size:   0,
			Hash:   SHA256OfNothing,
		})
	}

	return blocks, nil
}

// Validate validates the hash, if len(hash)>0.
func Validate(buf, hash []byte) bool {
	if len(hash) > 0 {
		hbuf := sha256.Sum256(buf)
		return bytes.Equal(hbuf[:], hash)
	}

	return true
}

type noopHash struct{}

func (noopHash) Sum32() uint32             { return 0 }
func (noopHash) BlockSize() int            { return 0 }
func (noopHash) Size() int                 { return 0 }
func (noopHash) Reset()                    {}
func (noopHash) Sum([]byte) []byte         { return nil }
func (noopHash) Write([]byte) (int, error) { return 0, nil }

type noopCounter struct{}

func (*noopCounter) Update(_ int64) {}
