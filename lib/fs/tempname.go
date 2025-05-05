// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/syncthing/syncthing/lib/build"
)

const (
	WindowsTempPrefix = "~syncthing~"
	UnixTempPrefix    = ".syncthing."
)

var tmpdir string

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
	var tdir, tname string
	if tmpdir != "" {
		tdir = tmpdir
	} else {
		tdir = filepath.Dir(name)
	}
	tbase := filepath.Base(name)
	if tmpdir != "" || len(tbase) > maxFilenameLength {
		// Hash the full name to prevent collisions if the same tbase
		// is being stored in different folders.
		tname = fmt.Sprintf("%s%x.tmp", prefix, sha256.Sum256([]byte(name)))
	} else {
		tname = prefix + tbase + ".tmp"
	}
	return filepath.Join(tdir, tname)
}

func TempName(name string) string {
	return TempNameWithPrefix(name, tempPrefix())
}

func SetTempDir(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return errors.New("not a directory")
	}
	perm := fi.Mode().Perm()
	if perm&(1<<(uint(7))) == 0 {
		return errors.New("tmpdir not writable")
	}
	tmpdir = dir
	return nil
}
