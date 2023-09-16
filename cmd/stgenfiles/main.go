// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

func main() {
	dir := flag.String("dir", "~/files", "Directory to generate into")
	files := flag.Int("files", 1000, "Number of files to create")
	maxExp := flag.Int("maxexp", 20, "Max size exponent")
	src := flag.String("src", "/dev/urandom", "Source of file data")
	flag.Parse()
	if err := generateFiles(*dir, *files, *maxExp, *src); err != nil {
		log.Println(err)
	}
}

func generateFiles(dir string, files, maxexp int, srcname string) error {
	fd, err := os.Open(srcname)
	if err != nil {
		return err
	}

	for i := 0; i < files; i++ {
		n := randomName()

		if rand.Float64() < 0.05 {
			// Some files and directories are dotfiles
			n = "." + n
		}

		p0 := filepath.Join(dir, string(n[0]), n[0:2])
		err = os.MkdirAll(p0, 0755)
		if err != nil {
			log.Fatal(err)
		}

		p1 := filepath.Join(p0, n)

		s := int64(1 << uint(rand.Intn(maxexp)))
		a := int64(128 * 1024)
		if a > s {
			a = s
		}
		s += rand.Int63n(a)

		if err := generateOneFile(fd, p1, s); err != nil {
			return err
		}
	}

	return nil
}

func generateOneFile(fd io.ReadSeeker, p1 string, s int64) error {
	src := io.LimitReader(&infiniteReader{fd}, s)
	dst, err := os.Create(p1)
	if err != nil {
		return err
	}

	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}

	err = dst.Close()
	if err != nil {
		return err
	}

	os.Chmod(p1, os.FileMode(rand.Intn(0777)|0400))

	t := time.Now().Add(-time.Duration(rand.Intn(30*86400)) * time.Second)
	return os.Chtimes(p1, t, t)
}

func randomName() string {
	var b [16]byte
	readRand(b[:])
	return fmt.Sprintf("%x", b[:])
}

func readRand(bs []byte) (int, error) {
	var r uint32
	for i := range bs {
		if i%4 == 0 {
			r = uint32(rand.Int63())
		}
		bs[i] = byte(r >> uint((i%4)*8))
	}
	return len(bs), nil
}

type infiniteReader struct {
	rd io.ReadSeeker
}

func (i *infiniteReader) Read(bs []byte) (int, error) {
	n, err := i.rd.Read(bs)
	if err == io.EOF {
		err = nil
		i.rd.Seek(0, 0)
	}
	return n, err
}
