//+build anal

package files

import (
	"crypto/md5"

	"github.com/calmh/syncthing/scanner"
)

type key struct {
	Name     string
	Version  uint64
	Modified int64
	Hash     [md5.Size]byte
}

func keyFor(f scanner.File) key {
	h := md5.New()
	for _, b := range f.Blocks {
		h.Write(b.Hash)
	}
	return key{
		Name:     f.Name,
		Version:  f.Version,
		Modified: f.Modified,
		Hash:     md5.Sum(nil),
	}
}

func (a key) newerThan(b key) bool {
	if a.Version != b.Version {
		return a.Version > b.Version
	}
	if a.Modified != b.Modified {
		return a.Modified > b.Modified
	}
	for i := 0; i < md5.Size; i++ {
		if a.Hash[i] != b.Hash[i] {
			return a.Hash[i] > b.Hash[i]
		}
	}
	return false
}
