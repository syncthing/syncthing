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
	if len(tbase) > 240 {
		hash := md5.New()
		hash.Write([]byte(name))
		tbase = fmt.Sprintf("%x", hash.Sum(nil))
	}
	tname := fmt.Sprintf("%s%s.tmp", t.prefix, tbase)
	return filepath.Join(tdir, tname)
}
