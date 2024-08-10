// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/syncthing/syncthing/lib/build"
)

const (
	WindowsTempPrefix = "~syncthing~"
	UnixTempPrefix    = ".syncthing."
)

func tempPrefix() string {
	if build.IsWindows {
		return WindowsTempPrefix
	} else {
		return UnixTempPrefix
	}
}

// Real filesystems usually handle 255 bytes. encfs has varying and
// confusing file name limits. We take a safe way out and switch to hashing
// quite early.
const maxFilenameLength = 160 - len(UnixTempPrefix) - len(".tmp")

// IsTemporary is true if the file name has the temporary prefix. Regardless
// of the normally used prefix, the standard Windows and Unix temp prefixes
// are always recognized as temp files.
func IsTemporary(name string) bool {
	name = filepath.Base(name)
	return strings.HasPrefix(name, WindowsTempPrefix) ||
		strings.HasPrefix(name, UnixTempPrefix)
}

func TempNameWithPrefix(name, prefix string) string {
	tdir := filepath.Dir(name)
	tbase := filepath.Base(name)
	var tname string
	if len(tbase) > maxFilenameLength {
		tname = fmt.Sprintf("%s%x.tmp", prefix, sha256.Sum256([]byte(tbase)))
	} else {
		tname = prefix + tbase + ".tmp"
	}
	return filepath.Join(tdir, tname)
}

func TempName(name string) string {
	return TempNameWithPrefix(name, tempPrefix())
}
