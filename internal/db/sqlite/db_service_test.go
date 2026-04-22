// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestBlobRange(t *testing.T) {
	exp := `
hash < x'249249'
hash >= x'249249' AND hash < x'492492'
hash >= x'492492' AND hash < x'6db6db'
hash >= x'6db6db' AND hash < x'924924'
hash >= x'924924' AND hash < x'b6db6d'
hash >= x'b6db6d' AND hash < x'db6db6'
hash >= x'db6db6'
	`

	ranges := blobRanges(7)
	buf := new(bytes.Buffer)
	for _, r := range ranges {
		fmt.Fprintln(buf, r.SQL("hash"))
	}

	if strings.TrimSpace(buf.String()) != strings.TrimSpace(exp) {
		t.Log(buf.String())
		t.Error("unexpected output")
	}
}
