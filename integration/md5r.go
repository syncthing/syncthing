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

var long bool

func main() {
	flag.BoolVar(&long, "l", false, "Long output")
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

	if !info.IsDir() {
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
