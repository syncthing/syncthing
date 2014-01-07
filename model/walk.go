package model

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/calmh/syncthing/protocol"
)

const BlockSize = 128 * 1024

type File struct {
	Name     string
	Flags    uint32
	Modified int64
	Version  uint32
	Blocks   []Block
}

func (f File) Size() (bytes int) {
	for _, b := range f.Blocks {
		bytes += int(b.Length)
	}
	return
}

func (f File) Equals(o File) bool {
	return f.Modified == o.Modified && f.Version == o.Version
}

func (f File) NewerThan(o File) bool {
	return f.Modified > o.Modified || (f.Modified == o.Modified && f.Version > o.Version)
}

func isTempName(name string) bool {
	return strings.HasPrefix(path.Base(name), ".syncthing.")
}

func tempName(name string, modified int64) string {
	tdir := path.Dir(name)
	tname := fmt.Sprintf(".syncthing.%s.%d", path.Base(name), modified)
	return path.Join(tdir, tname)
}

func (m *Model) genWalker(res *[]File, ign map[string][]string) filepath.WalkFunc {
	return func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if isTempName(p) {
			return nil
		}

		rn, err := filepath.Rel(m.dir, p)
		if err != nil {
			return nil
		}

		if pn, sn := path.Split(rn); sn == ".stignore" {
			pn := strings.Trim(pn, "/")
			bs, _ := ioutil.ReadFile(p)
			lines := bytes.Split(bs, []byte("\n"))
			var patterns []string
			for _, line := range lines {
				if len(line) > 0 {
					patterns = append(patterns, string(line))
				}
			}
			ign[pn] = patterns
			return nil
		}

		if info.Mode()&os.ModeType == 0 {
			fi, err := os.Stat(p)
			if err != nil {
				return nil
			}
			modified := fi.ModTime().Unix()

			m.RLock()
			hf, ok := m.local[rn]
			m.RUnlock()

			if ok && hf.Modified == modified {
				if nf := uint32(info.Mode()); nf != hf.Flags {
					hf.Flags = nf
					hf.Version++
				}
				*res = append(*res, hf)
			} else {
				m.Lock()
				if m.shouldSuppressChange(rn) {
					if m.trace["file"] {
						log.Println("FILE: SUPPRESS:", rn, m.fileWasSuppressed[rn], time.Since(m.fileLastChanged[rn]))
					}

					if ok {
						hf.Flags = protocol.FlagInvalid
						hf.Version++
						*res = append(*res, hf)
					}
					m.Unlock()
					return nil
				}
				m.Unlock()

				if m.trace["file"] {
					log.Printf("FILE: Hash %q", p)
				}
				fd, err := os.Open(p)
				if err != nil {
					if m.trace["file"] {
						log.Printf("FILE: %q: %v", p, err)
					}
					return nil
				}
				defer fd.Close()

				blocks, err := Blocks(fd, BlockSize)
				if err != nil {
					if m.trace["file"] {
						log.Printf("FILE: %q: %v", p, err)
					}
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
func (m *Model) Walk(followSymlinks bool) (files []File, ignore map[string][]string) {
	ignore = make(map[string][]string)
	fn := m.genWalker(&files, ignore)
	filepath.Walk(m.dir, fn)

	if followSymlinks {
		d, err := os.Open(m.dir)
		if err != nil {
			return
		}
		defer d.Close()

		fis, err := d.Readdir(-1)
		if err != nil {
			return
		}

		for _, fi := range fis {
			if fi.Mode()&os.ModeSymlink != 0 {
				filepath.Walk(path.Join(m.dir, fi.Name())+"/", fn)
			}
		}
	}

	return
}

// Walk returns the list of files found in the local repository by scanning the
// file system. Files are blockwise hashed. Patterns marked in .stignore files
// are removed from the results.
func (m *Model) FilteredWalk(followSymlinks bool) []File {
	var files, ignored = m.Walk(followSymlinks)
	return ignoreFilter(ignored, files)
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

func ignoreFilter(patterns map[string][]string, files []File) (filtered []File) {
nextFile:
	for _, f := range files {
		first, last := path.Split(f.Name)
		for prefix, pats := range patterns {
			if len(prefix) == 0 || prefix == first || strings.HasPrefix(first, prefix+"/") {
				for _, pattern := range pats {
					if match, _ := path.Match(pattern, last); match {
						continue nextFile
					}
				}
			}
		}
		filtered = append(filtered, f)
	}
	return filtered
}
