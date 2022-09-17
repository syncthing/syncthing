// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build ignore
// +build ignore

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"
)

func main() {
	flag.Parse()
	for _, arg := range flag.Args() {
		matches, err := filepath.Glob(arg)
		if err != nil {
			log.Fatal(err)
		}
		for _, file := range matches {
			if stat, err := os.Stat(file); err != nil {
				log.Fatal(err)
			} else if stat.IsDir() {
				err := filepath.Walk(file, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if filepath.Ext(path) == ".proto" {
						return formatProtoFile(path)
					}
					return nil
				})
				if err != nil {
					log.Fatal(err)
				}
			} else {
				if err := formatProtoFile(file); err != nil {
					log.Fatal(err)
				}
			}
		}
	}
}

func formatProtoFile(file string) error {
	log.Println("Formatting", file)
	in, err := os.Open(file)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(file + ".tmp")
	if err != nil {
		return err
	}
	defer out.Close()
	if err := formatProto(in, out); err != nil {
		return err
	}
	in.Close()
	out.Close()
	return os.Rename(file+".tmp", file)
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
