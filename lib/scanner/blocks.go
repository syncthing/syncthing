// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package scanner

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/syncthing/syncthing/lib/protocol"
)

var SHA256OfNothing = []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}

type Counter interface {
	Update(bytes int64)
}

// Blocks returns the blockwise hash of the reader.
func Blocks(r io.Reader, blocksize int, sizehint int64, counter Counter) ([]protocol.BlockInfo, error) {
	hf := sha256.New()
	hashLength := hf.Size()

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
			counter.Update(int64(n))
		}

		// Carve out a hash-sized chunk of "hashes" to store the hash for this
		// block.
		hashes = hf.Sum(hashes)
		thisHash, hashes = hashes[:hashLength], hashes[hashLength:]

		b := protocol.BlockInfo{
			Size:   int32(n),
			Offset: offset,
			Hash:   thisHash,
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
			Hash:   SHA256OfNothing,
		})
	}

	return blocks, nil
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
		if i >= len(src) || !bytes.Equal(tgt[i].Hash, src[i].Hash) {
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

		hash := hf.Sum(nil)
		hf.Reset()

		if !bytes.Equal(hash, block.Hash) {
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
	hash := hf.Sum(nil)

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
