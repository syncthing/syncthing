// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6_test

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if runtime.GOOS == "darwin" {
		vers, _ := exec.Command("sw_vers", "-productVersion").Output()
		if string(vers) == "10.8" || strings.HasPrefix(string(vers), "10.8.") {
			fmt.Fprintf(os.Stderr, "# skipping tests on OS X 10.8 to avoid kernel panics; golang.org/issue/17015\n")
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}
