// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package auto_test

import (
	"bytes"
	"testing"

	"github.com/syncthing/syncthing/internal/auto"
)

func TestAssets(t *testing.T) {
	assets := auto.Assets()
	idx, ok := assets["index.html"]
	if !ok {
		t.Fatal("No index.html in compiled in assets")
	}
	if !bytes.Contains(idx, []byte("<html")) {
		t.Fatal("No html in index.html")
	}
}
