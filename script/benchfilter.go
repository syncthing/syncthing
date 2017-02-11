// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build ignore

// Neatly format benchmarking output which otherwise looks like crap.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"
)

var (
	benchRe = regexp.MustCompile(`^(Bench[^\s]+)\s+(\d+)\s+(\d+ ns/op)\s*(\d+ B/op)?\s*(\d+ allocs/op)?`)
)

func main() {
	tw := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	br := bufio.NewScanner(os.Stdin)
	n := 0

	for br.Scan() {
		line := br.Text()

		if match := benchRe.FindStringSubmatch(line); match != nil {
			n++
			for i := range match[2:] {
				match[2+i] = fmt.Sprintf("%16s", match[2+i])
			}
			tw.Write([]byte(strings.Join(match[1:], "\t") + "\n"))
		} else if n > 0 && strings.HasPrefix(line, "ok") {
			n = 0
			tw.Flush()
			fmt.Printf("%s\n\n", line)
		}
	}
	tw.Flush()
}
