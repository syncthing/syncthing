// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	_ "github.com/syncthing/syncthing/lib/automaxprocs"
	"github.com/syncthing/syncthing/lib/sha256"
)

func main() {
	flag.Parse()
	log.Println(compareDirectories(flag.Args()...))
}

// Compare a number of directories. Returns nil if the contents are identical,
// otherwise an error describing the first found difference.
func compareDirectories(dirs ...string) error {
	chans := make([]chan fileInfo, len(dirs))
	for i := range chans {
		chans[i] = make(chan fileInfo)
	}
	errcs := make([]chan error, len(dirs))
	abort := make(chan struct{})

	for i := range dirs {
		errcs[i] = startWalker(dirs[i], chans[i], abort)
	}

	res := make([]fileInfo, len(dirs))
	for {
		numDone := 0
		for i := range chans {
			fi, ok := <-chans[i]
			if !ok {
				err, hasError := <-errcs[i]
				if hasError {
					close(abort)
					return err
				}
				numDone++
			}
			res[i] = fi
		}

		for i := 1; i < len(res); i++ {
			if res[i] != res[0] {
				close(abort)
				if res[i].name < res[0].name {
					return fmt.Errorf("%s missing %v (present in %s)", dirs[0], res[i], dirs[i])
				} else if res[i].name > res[0].name {
					return fmt.Errorf("%s missing %v (present in %s)", dirs[i], res[0], dirs[0])
				}
				return fmt.Errorf("mismatch; %v (%s) != %v (%s)", res[i], dirs[i], res[0], dirs[0])
			}
		}

		if numDone == len(dirs) {
			return nil
		}
	}
}

type fileInfo struct {
	name string
	mode os.FileMode
	mod  int64
	hash [sha256.Size]byte
}

func (f fileInfo) String() string {
	return fmt.Sprintf("%s %04o %d %x", f.name, f.mode, f.mod, f.hash)
}

func startWalker(dir string, res chan<- fileInfo, abort <-chan struct{}) chan error {
	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rn, _ := filepath.Rel(dir, path)
		if rn == "." {
			return nil
		}
		if rn == ".stversions" || rn == ".stfolder" {
			return filepath.SkipDir
		}

		var f fileInfo
		if info.Mode()&os.ModeSymlink != 0 {
			f = fileInfo{
				name: rn,
				mode: os.ModeSymlink,
			}

			tgt, err := os.Readlink(path)
			if err != nil {
				return err
			}
			f.hash = sha256.Sum256([]byte(tgt))
		} else if info.IsDir() {
			f = fileInfo{
				name: rn,
				mode: info.Mode(),
				// hash and modtime zero for directories
			}
		} else {
			f = fileInfo{
				name: rn,
				mode: info.Mode(),
				mod:  info.ModTime().Unix(),
			}
			sum, err := sha256file(path)
			if err != nil {
				return err
			}
			f.hash = sum
		}

		select {
		case res <- f:
			return nil
		case <-abort:
			return errors.New("abort")
		}
	}

	errc := make(chan error)
	go func() {
		err := filepath.Walk(dir, walker)
		close(res)
		if err != nil {
			errc <- err
		}
		close(errc)
	}()
	return errc
}

func sha256file(fname string) (hash [sha256.Size]byte, err error) {
	f, err := os.Open(fname)
	if err != nil {
		return
	}
	defer f.Close()

	h := sha256.New()
	io.Copy(h, f)
	hb := h.Sum(nil)
	copy(hash[:], hb)

	return
}
