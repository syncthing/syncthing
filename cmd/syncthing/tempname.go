// +build !windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

type tempNamer struct {
	prefix string
}

var defTempNamer = tempNamer{".syncthing"}

func (t tempNamer) IsTemporary(name string) bool {
	return strings.HasPrefix(filepath.Base(name), t.prefix)
}

func (t tempNamer) TempName(name string) string {
	tdir := filepath.Dir(name)
	tname := fmt.Sprintf("%s.%s", t.prefix, filepath.Base(name))
	return filepath.Join(tdir, tname)
}

func (t tempNamer) Hide(path string) error {
	return nil
}

func (t tempNamer) Show(path string) error {
	return nil
}
