package main

import (
	"bytes"
	"crypto/sha256"
	"io"
)

type BlockList []Block

type Block struct {
	Offset uint64
	Length uint32
	Hash   []byte
}

// Blocks returns the blockwise hash of the reader.
func Blocks(r io.Reader, blocksize int) (BlockList, error) {
	var blocks BlockList
	var offset uint64
	for {
		lr := &io.LimitedReader{r, int64(blocksize)}
		hf := sha256.New()
		n, err := io.Copy(hf, lr)
		if err != nil {
			return nil, err
		}

		if n == 0 {
			break
		}

		b := Block{
			Offset: offset,
			Length: uint32(n),
			Hash:   hf.Sum(nil),
		}
		blocks = append(blocks, b)
		offset += uint64(n)
	}

	return blocks, nil
}

// To returns the list of blocks necessary to transform src into dst.
// Both block lists must have been created with the same block size.
func (src BlockList) To(tgt BlockList) (have, need BlockList) {
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
