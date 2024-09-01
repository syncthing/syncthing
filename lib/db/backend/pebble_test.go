// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"fmt"
	"path"
	"testing"
)

func TestPebbleBackendBehavior(t *testing.T) {
	next := 0
	opener := func() Backend {
		next++
		be, err := OpenPebble(path.Join(t.TempDir(), fmt.Sprintf("db%d", next)))
		if err != nil {
			t.Fatal(err)
		}
		return be
	}
	testBackendBehavior(t, opener)
}
