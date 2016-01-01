// Copyright (C) 2016 The Protocol Authors.

package protocol

import "fmt"

type HashAlgorithm int

const (
	SHA256 HashAlgorithm = iota
)

func (h HashAlgorithm) String() string {
	switch h {
	case SHA256:
		return "sha256"
	default:
		return "unknown"
	}
}

// FlagBits returns the bits that we should or into the folder flag field to
// indicate the hash algorithm.
func (h HashAlgorithm) FlagBits() uint32 {
	switch h {
	case SHA256:
		return FolderHashSHA256 << FolderHashShiftBits
	default:
		panic("unknown hash algorithm")
	}
}

func (h *HashAlgorithm) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "sha256":
		*h = SHA256
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
	default:
		return 0, fmt.Errorf("Unknown hash algorithm %d", algo)
	}
}
