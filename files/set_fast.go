//+build !anal

package files

import "github.com/calmh/syncthing/scanner"

type key struct {
	Name    string
	Version uint64
}

func keyFor(f scanner.File) key {
	return key{
		Name:    f.Name,
		Version: f.Version,
	}
}

func (a key) newerThan(b key) bool {
	return a.Version > b.Version
}
