// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package auto_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"

	"github.com/syncthing/syncthing/lib/api/auto"
)

func TestAssets(t *testing.T) {
	assets := auto.Assets()
	idx, ok := assets["default/index.html"]
	if !ok {
		t.Fatal("No index.html in compiled in assets")
	}
	if !idx.Gzipped {
		t.Fatal("default/index.html should be compressed")
	}

	var gr *gzip.Reader
	gr, _ = gzip.NewReader(strings.NewReader(idx.Content))
	html, _ := io.ReadAll(gr)

	if !bytes.Contains(html, []byte("<html")) {
		t.Fatal("No html in index.html")
	}
}
