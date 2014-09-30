// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build windows

package model

import (
	"fmt"
	"path/filepath"
	"strings"
)

type tempNamer struct {
	prefix string
}

var defTempNamer = tempNamer{"~syncthing~"}

func (t tempNamer) IsTemporary(name string) bool {
	return strings.HasPrefix(filepath.Base(name), t.prefix)
}

func (t tempNamer) TempName(name string) string {
	tdir := filepath.Dir(name)
	tname := fmt.Sprintf("%s.%s.tmp", t.prefix, filepath.Base(name))
	return filepath.Join(tdir, tname)
}
