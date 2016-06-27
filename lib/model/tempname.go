// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"crypto/md5"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

type tempNamer struct {
	prefix string
}

var defTempNamer tempNamer

// (max filename length supported by FS) - len(".syncthing.") - len(".tmp")
// Worst case is EncFS which according to man page supports filenames up to
// "approximately" 3*(N-2)/4 characters where N is a limit of underlying FS.
// If underlying FS is ext4 in practice it means that maximum filename length
// is 188 characters. Minus len(".syncthing."), minus len(".tmp") gives:
const maxFilenameLength = 173

func init() {
	if runtime.GOOS == "windows" {
		defTempNamer = tempNamer{"~syncthing~"}
	} else {
		defTempNamer = tempNamer{".syncthing."}
	}
}

func (t tempNamer) IsTemporary(name string) bool {
	return strings.HasPrefix(filepath.Base(name), t.prefix)
}

func (t tempNamer) TempName(name string) string {
	tdir := filepath.Dir(name)
	tbase := filepath.Base(name)
	if len(tbase) > maxFilenameLength {
		hash := md5.New()
		hash.Write([]byte(name))
		tbase = fmt.Sprintf("%x", hash.Sum(nil))
	}
	tname := fmt.Sprintf("%s%s.tmp", t.prefix, tbase)
	return filepath.Join(tdir, tname)
}
