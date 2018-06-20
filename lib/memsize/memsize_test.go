// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package memsize

import (
	"testing"
)

func TestMemsize(t *testing.T) {
	if _, err := MemorySize(); err != nil {
		t.Error("MemorySize failed with error", err)
	}
}
