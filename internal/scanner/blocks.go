// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package scanner

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/syncthing/syncthing/internal/protocol"
)

var sha256OfNothing = []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}

// Blocks returns the blockwise hash of the reader.
func Blocks(r io.Reader, blocksize int, sizehint int64) ([]protocol.BlockInfo, error) {
	var blocks []protocol.BlockInfo
	if sizehint > 0 {
		blocks = make([]protocol.BlockInfo, 0, int(sizehint/int64(blocksize)))
	}
	var offset int64
	hf := sha256.New()
	for {
		lr := &io.LimitedReader{R: r, N: int64(blocksize)}
		n, err := io.Copy(hf, lr)
		if err != nil {
			return nil, err
		}

		if n == 0 {
			break
		}

		b := protocol.BlockInfo{
			Size:   uint32(n),
			Offset: offset,
			Hash:   hf.Sum(nil),
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
			Hash:   sha256OfNothing,
		})
	}

	return blocks, nil
}

// BlockDiff returns lists of common and missing (to transform src into tgt)
// blocks. Both block lists must have been created with the same block size.
func BlockDiff(src, tgt []protocol.BlockInfo) (have, need []protocol.BlockInfo) {
	if len(tgt) == 0 && len(src) != 0 {
		return nil, nil
	}

	// Set the Offset field on each target block
	var offset int64
	for i := range tgt {
		tgt[i].Offset = offset
		offset += int64(tgt[i].Size)
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

		hash := hf.Sum(nil)
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
