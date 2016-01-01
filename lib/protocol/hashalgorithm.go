// Copyright (C) 2015 The Protocol Authors.

package protocol

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"hash"

	"github.com/spaolacci/murmur3"
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

// New returns a new hash.Hash for the given algorithm.
func (h HashAlgorithm) New() hash.Hash {
	var hf hash.Hash
	switch h {
	case SHA256:
		hf = sha256.New()
	case Murmur3:
		hf = murmur3.New128()
	default:
		panic("unknown hash algorithm")
	}
	return hf
}

// Secure returns true if the hash algorithm is known to be cryptographically
// secure.
func (h HashAlgorithm) Secure() bool {
	switch h {
	case SHA256:
		return true
	default:
		return false
	}
}

// FlagBits returns the bits that we should or into the folder flag field to
// indicate the hash algorithm.
func (h HashAlgorithm) FlagBits() uint32 {
	switch h {
	case SHA256:
		return FolderHashSHA256 << FolderHashShiftBits
	case Murmur3:
		return FolderHashMurmur3 << FolderHashShiftBits
	default:
		panic("unknown hash algorithm")
	}
}

var sha256OfEmptyBlock = sha256.Sum256(make([]byte, BlockSize))

func (h HashAlgorithm) Empty(b BlockInfo) bool {
	switch h {
	case SHA256:
		return b.Size == BlockSize && bytes.Equal(b.Hash, sha256OfEmptyBlock[:])
	default:
		// No other algorithm is currently capable of deciding this for sure.
		return false
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
	return fmt.Errorf("Unknown hash algorithm %q", string(bs))
}

func (h *HashAlgorithm) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

func HashAlgorithmFromFlagBits(flags uint32) (HashAlgorithm, error) {
	algo := flags >> FolderHashShiftBits & FolderHashMask
	switch algo {
	case FolderHashSHA256:
		return SHA256, nil
	case FolderHashMurmur3:
		return Murmur3, nil
	default:
		return 0, fmt.Errorf("Unknown hash algorithm %d", algo)
	}
}
