package model

import (
	"fmt"
	"log"
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
	Blocks   []Block
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

func (m *Model) genWalker(res *[]File) filepath.WalkFunc {
	return func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if isTempName(p) {
			return nil
		}

		if info.Mode()&os.ModeType == 0 {
			rn, err := filepath.Rel(m.dir, p)
			if err != nil {
				return nil
			}

			fi, err := os.Stat(p)
			if err != nil {
				return nil
			}
			modified := fi.ModTime().Unix()

			m.RLock()
			hf, ok := m.local[rn]
			m.RUnlock()

			if ok && hf.Modified == modified {
				// No change
				*res = append(*res, hf)
			} else {
				if m.trace["file"] {
					log.Printf("FILE: Hash %q", p)
				}
				fd, err := os.Open(p)
				if err != nil {
					return nil
				}
				defer fd.Close()

				blocks, err := Blocks(fd, BlockSize)
				if err != nil {
					return nil
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

// Walk returns the list of files found in the local repository by scanning the
// file system. Files are blockwise hashed.
func (m *Model) Walk(followSymlinks bool) []File {
	var files []File
	fn := m.genWalker(&files)
	filepath.Walk(m.dir, fn)

	if followSymlinks {
		d, err := os.Open(m.dir)
		if err != nil {
			return files
		}
		defer d.Close()

		fis, err := d.Readdir(-1)
		if err != nil {
			return files
		}

		for _, fi := range fis {
			if fi.Mode()&os.ModeSymlink != 0 {
				filepath.Walk(path.Join(m.dir, fi.Name())+"/", fn)
			}
		}
	}

	return files
}

func (m *Model) cleanTempFile(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeType == 0 && isTempName(path) {
		if m.trace["file"] {
			log.Printf("FILE: Remove %q", path)
		}
		os.Remove(path)
	}
	return nil
}

func (m *Model) cleanTempFiles() {
	filepath.Walk(m.dir, m.cleanTempFile)
}
