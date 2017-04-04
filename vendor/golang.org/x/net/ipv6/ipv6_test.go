// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"fmt"
	"os"
	"testing"
)

var disableTests = false

func TestMain(m *testing.M) {
	if disableTests {
		fmt.Fprintf(os.Stderr, "ipv6 tests disabled in Go 1.9 until netreflect is fixed (Issue 19051)\n")
		os.Exit(0)
	}
	// call flag.Parse() here if TestMain uses flags
	os.Exit(m.Run())
}
