// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !windows

package model

import (
	"crypto/md5"
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
	hash := md5.New()
	hash.Write([]byte(name))
	tdir := filepath.Dir(name)
	tname := fmt.Sprintf("%s.%x", t.prefix, hash.Sum(nil))
	return filepath.Join(tdir, tname)
}
