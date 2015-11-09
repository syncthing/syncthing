// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package scanner

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"sync/atomic"

	"github.com/spaolacci/murmur3"
	"github.com/syncthing/syncthing/lib/protocol"
)

type HashAlgorithm int

const (
	SHA256 HashAlgorithm = iota
	Murmur3
)

func (h HashAlgorithm) String() string {
	switch h {
	case SHA256:
		return "sha256"
	case Murmur3:
		return "murmur3"
	default:
		return "unknown"
	}
}

func (h *HashAlgorithm) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "sha256":
		*h = SHA256
		return nil
	case "murmur3":
		*h = Murmur3
		return nil
	}
	return errors.New("unknown hash algorithm")
}

func (h *HashAlgorithm) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

// Blocks returns the blockwise hash of the reader.
func Blocks(algo HashAlgorithm, r io.Reader, blocksize int, sizehint int64, counter *int64) ([]protocol.BlockInfo, error) {
	var hf hash.Hash
	switch algo {
	case SHA256:
		hf = sha256.New()
	case Murmur3:
		hf = murmur3.New128()
	default:
		panic("unknown hash algorithm")
	}
	hashLength := hf.Size()

	var blocks []protocol.BlockInfo
	var hashes, thisHash []byte

	if sizehint > 0 {
		// Allocate contiguous blocks for the BlockInfo structures and their
		// hashes once and for all.
		numBlocks := int(sizehint / int64(blocksize))
		blocks = make([]protocol.BlockInfo, 0, numBlocks)
		hashes = make([]byte, 0, hashLength*numBlocks)
	}

	// A 32k buffer is used for copying into the hash function.
	buf := make([]byte, 32<<10)

	var offset int64
	for {
		lr := io.LimitReader(r, int64(blocksize))
		n, err := copyBuffer(hf, lr, buf)
		if err != nil {
			return nil, err
		}

		if n == 0 {
			break
		}

		if counter != nil {
			atomic.AddInt64(counter, int64(n))
		}

		// Carve out a hash-sized chunk of "hashes" to store the hash for this
		// block.
		hashes = hf.Sum(hashes)
		thisHash, hashes = hashes[:hashLength], hashes[hashLength:]

		b := protocol.BlockInfo{
			Size:   int32(n),
			Offset: offset,
			Hash:   sum256(hf),
		}

		blocks = append(blocks, b)
		offset += int64(n)

		hf.Reset()
	}

	if len(blocks) == 0 {
		// Empty file
		blocks = append(blocks, protocol.BlockInfo{
			Offset: 0,
			Size:   0,
			Hash:   sum256(hf),
		})
	}

	return blocks, nil
}

// This returns a 256 bit (32 byte) hash, regardless of the size of the hash
// function. We make assumptions in a number of places that hashes are 32
// bytes long...
func sum256(hf hash.Hash) []byte {
	h := hf.Sum(nil)
	if len(h) > 32 {
		panic("unsupported hash length > 256 bits")
	}
	if len(h) == 32 {
		return h
	}
	h256 := make([]byte, 32)
	copy(h256, h)
	return h256
}

// PopulateOffsets sets the Offset field on each block
func PopulateOffsets(blocks []protocol.BlockInfo) {
	var offset int64
	for i := range blocks {
		blocks[i].Offset = offset
		offset += int64(blocks[i].Size)
	}
}

// BlockDiff returns lists of common and missing (to transform src into tgt)
// blocks. Both block lists must have been created with the same block size.
func BlockDiff(src, tgt []protocol.BlockInfo) (have, need []protocol.BlockInfo) {
	if len(tgt) == 0 && len(src) != 0 {
		return nil, nil
	}

	if len(tgt) != 0 && len(src) == 0 {
		// Copy the entire file
		return nil, tgt
	}

	for i := range tgt {
		if i >= len(src) || bytes.Compare(tgt[i].Hash, src[i].Hash) != 0 {
			// Copy differing block
			need = append(need, tgt[i])
		} else {
			have = append(have, tgt[i])
		}
	}

	return have, need
}

// Verify returns nil or an error describing the mismatch between the block
// list and actual reader contents
func Verify(r io.Reader, blocksize int, blocks []protocol.BlockInfo) error {
	hf := sha256.New()
	for i, block := range blocks {
		lr := &io.LimitedReader{R: r, N: int64(blocksize)}
		_, err := io.Copy(hf, lr)
		if err != nil {
			return err
		}

		hash := sum256(hf)
		hf.Reset()

		if bytes.Compare(hash, block.Hash) != 0 {
			return fmt.Errorf("hash mismatch %x != %x for block %d", hash, block.Hash, i)
		}
	}

	// We should have reached the end  now
	bs := make([]byte, 1)
	n, err := r.Read(bs)
	if n != 0 || err != io.EOF {
		return fmt.Errorf("file continues past end of blocks")
	}

	return nil
}

func VerifyBuffer(buf []byte, block protocol.BlockInfo) ([]byte, error) {
	if len(buf) != int(block.Size) {
		return nil, fmt.Errorf("length mismatch %d != %d", len(buf), block.Size)
	}
	hf := sha256.New()
	_, err := hf.Write(buf)
	if err != nil {
		return nil, err
	}
	hash := sum256(hf)

	if !bytes.Equal(hash, block.Hash) {
		return hash, fmt.Errorf("hash mismatch %x != %x", hash, block.Hash)
	}

	return hash, nil
}

// BlocksEqual returns whether two slices of blocks are exactly the same hash
// and index pair wise.
func BlocksEqual(src, tgt []protocol.BlockInfo) bool {
	if len(tgt) != len(src) {
		return false
	}

	for i, sblk := range src {
		if !bytes.Equal(sblk.Hash, tgt[i].Hash) {
			return false
		}
	}
	return true
}

// This is a copy & paste of io.copyBuffer from the Go 1.5 standard library,
// as we want this but also want to build with Go 1.3+.

// copyBuffer is the actual implementation of Copy and CopyBuffer.
// if buf is nil, one is allocated.
func copyBuffer(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}
	if buf == nil {
		buf = make([]byte, 32*1024)
	}
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return written, err
}
