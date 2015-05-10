// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"strings"
	"testing"
)

func TestLongTempFilename(t *testing.T) {
	filename := ""
	for i := 0; i < 300; i++ {
		filename += "l"
	}
	tFile := defTempNamer.TempName(filename)
	if len(tFile) < 10 || len(tFile) > 200 {
		t.Fatal("Invalid long filename")
	}
	if !strings.HasSuffix(defTempNamer.TempName("short"), "short.tmp") {
		t.Fatal("Invalid short filename", defTempNamer.TempName("short"))
	}
}
