// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	mr "math/rand"
	"os"
	"path/filepath"
	"time"
)

func name() string {
	var b [16]byte
	rand.Reader.Read(b[:])
	return fmt.Sprintf("%x", b[:])
}

func main() {
	var files int
	var maxexp int
	var srcname string

	flag.IntVar(&files, "files", 1000, "Number of files")
	flag.IntVar(&maxexp, "maxexp", 20, "Maximum file size (max = 2^n + 128*1024 B)")
	flag.StringVar(&srcname, "src", "/usr/share/dict/words", "Source material")
	flag.Parse()

	fd, err := os.Open(srcname)
	if err != nil {
		log.Fatal(err)
	}

	for i := 0; i < files; i++ {
		n := name()
		p0 := filepath.Join(string(n[0]), n[0:2])
		err = os.MkdirAll(p0, 0755)
		if err != nil {
			log.Fatal(err)
		}

		s := 1 << uint(mr.Intn(maxexp))
		a := 128 * 1024
		if a > s {
			a = s
		}
		s += mr.Intn(a)

		src := io.LimitReader(&inifiteReader{fd}, int64(s))

		p1 := filepath.Join(p0, n)
		dst, err := os.Create(p1)
		if err != nil {
			log.Fatal(err)
		}

		_, err = io.Copy(dst, src)
		if err != nil {
			log.Fatal(err)
		}

		err = dst.Close()
		if err != nil {
			log.Fatal(err)
		}

		err = os.Chmod(p1, os.FileMode(mr.Intn(0777)|0400))
		if err != nil {
			log.Fatal(err)
		}

		t := time.Now().Add(-time.Duration(mr.Intn(30*86400)) * time.Second)
		err = os.Chtimes(p1, t, t)
		if err != nil {
			log.Fatal(err)
		}
	}
}

type inifiteReader struct {
	rd io.ReadSeeker
}

func (i *inifiteReader) Read(bs []byte) (int, error) {
	n, err := i.rd.Read(bs)
	if err == io.EOF {
		err = nil
		i.rd.Seek(0, 0)
	}
	return n, err
}
