// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/syncthing/syncthing/lib/sha256"
)

const (
	WindowsTempPrefix = "~syncthing~"
	UnixTempPrefix    = ".syncthing."
)

var TempPrefix string

// Real filesystems usually handle 255 bytes. encfs has varying and
// confusing file name limits. We take a safe way out and switch to hashing
// quite early.
const maxFilenameLength = 160 - len(UnixTempPrefix) - len(".tmp")

func init() {
	if runtime.GOOS == "windows" {
		TempPrefix = WindowsTempPrefix
	} else {
		TempPrefix = UnixTempPrefix
	}
}

// IsTemporary is true if the file name has the temporary prefix. Regardless
// of the normally used prefix, the standard Windows and Unix temp prefixes
// are always recognized as temp files.
func IsTemporary(name string) bool {
	name = filepath.Base(name)
	if strings.HasPrefix(name, WindowsTempPrefix) ||
		strings.HasPrefix(name, UnixTempPrefix) {
		return true
	}
	return false
}

func TempNameWithPrefix(name, prefix string) string {
	tdir := filepath.Dir(name)
	tbase := filepath.Base(name)
	if len(tbase) > maxFilenameLength {
		tbase = fmt.Sprintf("%x", sha256.Sum256([]byte(name)))
	}
	tname := fmt.Sprintf("%s%s.tmp", prefix, tbase)
	return filepath.Join(tdir, tname)
}

func TempName(name string) string {
	return TempNameWithPrefix(name, TempPrefix)
}
