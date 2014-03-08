package main

import (
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

type tempNamer struct {
	prefix string
}

var defTempNamer = tempNamer{".syncthing"}

func (t tempNamer) IsTemporary(name string) bool {
	if runtime.GOOS == "windows" {
		name = filepath.ToSlash(name)
	}
	return strings.HasPrefix(path.Base(name), t.prefix)
}

func (t tempNamer) TempName(name string) string {
	tdir := path.Dir(name)
	tname := fmt.Sprintf("%s.%s", t.prefix, path.Base(name))
	return path.Join(tdir, tname)
}
