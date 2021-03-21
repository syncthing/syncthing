// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//+build noassets

package auto

import (
	"bytes"
	"compress/gzip"

	"github.com/syncthing/syncthing/lib/assets"
)

func Assets() map[string]assets.Asset {
	// Return a minimal index.html and nothing else, to allow the trivial
	// test to pass.

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	_, _ = gw.Write([]byte("<html></html>"))
	_ = gw.Flush()
	return map[string]assets.Asset{
		"default/index.html": {
			Gzipped: true,
			Content: buf.String(),
		},
	}
}
