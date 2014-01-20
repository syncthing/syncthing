package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"io/ioutil"
	mr "math/rand"
	"os"
	"path"
)

func name() string {
	var b [16]byte
	rand.Reader.Read(b[:])
	return fmt.Sprintf("%x", b[:])
}

func main() {
	var files int
	var maxsize int

	flag.IntVar(&files, "files", 1000, "Number of files")
	flag.IntVar(&maxsize, "maxsize", 1000, "Maximum file size (KB)")
	flag.Parse()

	for i := 0; i < files; i++ {
		n := name()
		p0 := path.Join(string(n[0]), n[0:2])
		os.MkdirAll(p0, 0755)
		s := mr.Intn(maxsize * 1024)
		b := make([]byte, s)
		rand.Reader.Read(b)
		p1 := path.Join(p0, n)
		ioutil.WriteFile(p1, b, 0644)
	}
}
