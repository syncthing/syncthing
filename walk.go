package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const BlockSize = 128 * 1024

type File struct {
	Name     string
	Flags    uint32
	Modified int64
	Blocks   BlockList
}

func (f File) Dump() {
	fmt.Printf("%s\n", f.Name)
	for _, b := range f.Blocks {
		fmt.Printf("  %dB @ %d: %x\n", b.Length, b.Offset, b.Hash)
	}
	fmt.Println()
}

func (f File) Size() (bytes int) {
	for _, b := range f.Blocks {
		bytes += int(b.Length)
	}
	return
}

func isTempName(name string) bool {
	return strings.HasPrefix(path.Base(name), ".syncthing.")
}

func tempName(name string, modified int64) string {
	tdir := path.Dir(name)
	tname := fmt.Sprintf(".syncthing.%s.%d", path.Base(name), modified)
	return path.Join(tdir, tname)
}

func genWalker(base string, res *[]File, model *Model) filepath.WalkFunc {
	return func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if isTempName(p) {
			return nil
		}

		if info.Mode()&os.ModeType == 0 {
			rn, err := filepath.Rel(base, p)
			if err != nil {
				return err
			}

			fi, err := os.Stat(p)
			if err != nil {
				return err
			}
			modified := fi.ModTime().Unix()

			hf, ok := model.LocalFile(rn)
			if ok && hf.Modified == modified {
				// No change
				*res = append(*res, hf)
			} else {
				if opts.Debug.TraceFile {
					debugf("FILE: Hash %q", p)
				}
				fd, err := os.Open(p)
				if err != nil {
					return err
				}
				defer fd.Close()

				blocks, err := Blocks(fd, BlockSize)
				if err != nil {
					return err
				}
				f := File{
					Name:     rn,
					Flags:    uint32(info.Mode()),
					Modified: modified,
					Blocks:   blocks,
				}
				*res = append(*res, f)
			}
		}

		return nil
	}
}

func Walk(dir string, model *Model, followSymlinks bool) []File {
	var files []File
	fn := genWalker(dir, &files, model)
	err := filepath.Walk(dir, fn)
	if err != nil {
		warnln(err)
	}

	if !opts.NoSymlinks {
		d, err := os.Open(dir)
		if err != nil {
			warnln(err)
			return files
		}
		defer d.Close()

		fis, err := d.Readdir(-1)
		if err != nil {
			warnln(err)
			return files
		}

		for _, fi := range fis {
			if fi.Mode()&os.ModeSymlink != 0 {
				err := filepath.Walk(path.Join(dir, fi.Name())+"/", fn)
				if err != nil {
					warnln(err)
				}
			}
		}
	}

	return files
}

func cleanTempFile(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeType == 0 && isTempName(path) {
		if opts.Debug.TraceFile {
			debugf("FILE: Remove %q", path)
		}
		os.Remove(path)
	}
	return nil
}

func CleanTempFiles(dir string) {
	filepath.Walk(dir, cleanTempFile)
}
