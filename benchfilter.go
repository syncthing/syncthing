// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build ignore

// Neatly format benchmarking output which otherwise looks like crap.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"text/tabwriter"
)

var (
	benchRe   = regexp.MustCompile(`^Bench`)
	spacesRe  = regexp.MustCompile(`\s+`)
	numbersRe = regexp.MustCompile(`\b[\d\.]+\b`)
)

func main() {
	tw := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	br := bufio.NewScanner(os.Stdin)
	n := 0

	for br.Scan() {
		line := br.Bytes()

		if benchRe.Match(line) {
			n++
			line = spacesRe.ReplaceAllLiteral(line, []byte("\t"))
			line = numbersRe.ReplaceAllFunc(line, func(n []byte) []byte {
				return []byte(fmt.Sprintf("%12s", n))
			})
			tw.Write(line)
			tw.Write([]byte("\n"))
		} else if n > 0 && bytes.HasPrefix(line, []byte("ok")) {
			n = 0
			tw.Flush()
			fmt.Printf("%s\n\n", line)
		}
	}
	tw.Flush()
}
