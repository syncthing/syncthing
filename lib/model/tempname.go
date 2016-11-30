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
	prefix    string
	recognize []string
}

const (
	windowsTempPrefix = "~syncthing~"
	unixTempPrefix    = ".syncthing."
)

var defTempNamer tempNamer

// Real filesystems usually handle 255 bytes. encfs has varying and
// confusing file name limits. We take a safe way out and switch to hashing
// quite early.
const maxFilenameLength = 160 - len(unixTempPrefix) - len(".tmp")

func init() {
	if runtime.GOOS == "windows" {
		defTempNamer = tempNamer{windowsTempPrefix, []string{unixTempPrefix, windowsTempPrefix}}
	} else {
		defTempNamer = tempNamer{unixTempPrefix, []string{unixTempPrefix, windowsTempPrefix}}
	}
}

// IsTemporary is true if the file name has the temporary prefix. Regardless
// of the normally used prefix, the standard Windows and Unix temp prefixes
// are always recognized as temp files.
func (t tempNamer) IsTemporary(name string) bool {
	name = filepath.Base(name)
	for _, prefix := range t.recognize {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
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
