// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build ignore

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

func ReadRand(bs []byte) (int, error) {
	var r uint32
	for i := range bs {
		if i%4 == 0 {
			r = uint32(rand.Int63())
		}
		bs[i] = byte(r >> uint((i%4)*8))
	}
	return len(bs), nil
}

func name() string {
	var b [16]byte
	ReadRand(b[:])
	return fmt.Sprintf("%x", b[:])
}

func main() {
	var files int
	var maxexp int
	var srcname string
	var random bool

	flag.IntVar(&files, "files", 1000, "Number of files")
	flag.IntVar(&maxexp, "maxexp", 20, "Maximum file size (max = 2^n + 128*1024 B)")
	flag.StringVar(&srcname, "src", "/usr/share/dict/words", "Source material")
	flag.BoolVar(&random, "random", true, "When false, always generate the same set of file")
	flag.Parse()

	if random {
		rand.Seed(time.Now().UnixNano())
	} else {
		rand.Seed(42)
	}

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

		s := 1 << uint(rand.Intn(maxexp))
		a := 128 * 1024
		if a > s {
			a = s
		}
		s += rand.Intn(a)

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

		err = os.Chmod(p1, os.FileMode(rand.Intn(0777)|0400))
		if err != nil {
			log.Fatal(err)
		}

		t := time.Now().Add(-time.Duration(rand.Intn(30*86400)) * time.Second)
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
