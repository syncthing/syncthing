// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build ignore

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"
)

func main() {
	flag.Parse()
	file := flag.Arg(0)
	in, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	out, err := os.Create(file + ".tmp")
	if err != nil {
		log.Fatal(err)
	}

	if err := formatProto(in, out); err != nil {
		log.Fatal(err)
	}
	in.Close()
	out.Close()
	if err := os.Rename(file+".tmp", file); err != nil {
		log.Fatal(err)
	}
}

func formatProto(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	lineExp := regexp.MustCompile(`([^=]+)\s+([^=\s]+?)\s*=(.+)`)
	var tw *tabwriter.Writer
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "//") {
			if _, err := fmt.Fprintln(out, line); err != nil {
				return err
			}
			continue
		}

		ms := lineExp.FindStringSubmatch(line)
		for i := range ms {
			ms[i] = strings.TrimSpace(ms[i])
		}
		if len(ms) == 4 && ms[1] != "option" {
			typ := strings.Join(strings.Fields(ms[1]), " ")
			name := ms[2]
			id := ms[3]
			if tw == nil {
				tw = tabwriter.NewWriter(out, 4, 4, 1, ' ', 0)
			}
			if typ == "" {
				// We're in an enum
				fmt.Fprintf(tw, "\t%s\t= %s\n", name, id)
			} else {
				// Message
				fmt.Fprintf(tw, "\t%s\t%s\t= %s\n", typ, name, id)
			}
		} else {
			if tw != nil {
				if err := tw.Flush(); err != nil {
					return err
				}
				tw = nil
			}
			if _, err := fmt.Fprintln(out, line); err != nil {
				return err
			}
		}
	}

	return nil
}
