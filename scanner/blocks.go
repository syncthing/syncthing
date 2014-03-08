package scanner

import (
	"bytes"
	"crypto/sha256"
	"io"
)

type Block struct {
	Offset int64
	Size   uint32
	Hash   []byte
}

// Blocks returns the blockwise hash of the reader.
func Blocks(r io.Reader, blocksize int) ([]Block, error) {
	var blocks []Block
	var offset int64
	for {
		lr := &io.LimitedReader{R: r, N: int64(blocksize)}
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
			Size:   uint32(n),
			Hash:   hf.Sum(nil),
		}
		blocks = append(blocks, b)
		offset += int64(n)
	}

	if len(blocks) == 0 {
		// Empty file
		blocks = append(blocks, Block{
			Offset: 0,
			Size:   0,
			Hash:   []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55},
		})
	}

	return blocks, nil
}

// BlockDiff returns lists of common and missing (to transform src into tgt)
// blocks. Both block lists must have been created with the same block size.
func BlockDiff(src, tgt []Block) (have, need []Block) {
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
