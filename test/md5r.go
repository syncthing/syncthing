// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// +build ignore

package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var (
	long bool
	dirs bool
)

func main() {
	flag.BoolVar(&long, "l", false, "Long output")
	flag.BoolVar(&dirs, "d", false, "Check dirs")
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		args = []string{"."}
	}

	for _, path := range args {
		err := filepath.Walk(path, walker)

		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func walker(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if dirs && info.IsDir() {
		fmt.Printf("%s  %s 0%03o %d\n", "-", path, info.Mode(), info.ModTime().Unix())
	} else if !info.IsDir() {
		sum, err := md5file(path)
		if err != nil {
			return err
		}
		if long {
			fmt.Printf("%s  %s 0%03o %d\n", sum, path, info.Mode(), info.ModTime().Unix())
		} else {
			fmt.Printf("%s  %s\n", sum, path)
		}
	}

	return nil
}

func md5file(fname string) (hash string, err error) {
	f, err := os.Open(fname)
	if err != nil {
		return
	}
	defer f.Close()

	h := md5.New()
	io.Copy(h, f)
	hb := h.Sum(nil)
	hash = fmt.Sprintf("%x", hb)

	return
}
