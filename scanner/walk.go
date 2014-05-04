package scanner

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/calmh/syncthing/lamport"
	"github.com/calmh/syncthing/protocol"
)

type Walker struct {
	// Dir is the base directory for the walk
	Dir string
	// BlockSize controls the size of the block used when hashing.
	BlockSize int
	// If IgnoreFile is not empty, it is the name used for the file that holds ignore patterns.
	IgnoreFile string
	// If TempNamer is not nil, it is used to ignore tempory files when walking.
	TempNamer TempNamer
	// If CurrentFiler is not nil, it is queried for the current file before rescanning.
	CurrentFiler CurrentFiler
	// If Suppressor is not nil, it is queried for supression of modified files.
	// Suppressed files will be returned with empty metadata and the Suppressed flag set.
	// Requires CurrentFiler to be set.
	Suppressor Suppressor

	suppressed map[string]bool // file name -> suppression status
}

type TempNamer interface {
	// Temporary returns a temporary name for the filed referred to by filepath.
	TempName(path string) string
	// IsTemporary returns true if path refers to the name of temporary file.
	IsTemporary(path string) bool
}

type Suppressor interface {
	// Supress returns true if the update to the named file should be ignored.
	Suppress(name string, fi os.FileInfo) bool
}

type CurrentFiler interface {
	// CurrentFile returns the file as seen at last scan.
	CurrentFile(name string) File
}

// Walk returns the list of files found in the local repository by scanning the
// file system. Files are blockwise hashed.
func (w *Walker) Walk() (files []File, ignore map[string][]string, err error) {
	w.lazyInit()

	if debug {
		dlog.Println("Walk", w.Dir, w.BlockSize, w.IgnoreFile)
	}

	err = checkDir(w.Dir)
	if err != nil {
		return
	}

	t0 := time.Now()

	ignore = make(map[string][]string)
	hashFiles := w.walkAndHashFiles(&files, ignore)

	filepath.Walk(w.Dir, w.loadIgnoreFiles(w.Dir, ignore))
	filepath.Walk(w.Dir, hashFiles)

	if debug {
		t1 := time.Now()
		d := t1.Sub(t0).Seconds()
		dlog.Printf("Walk in %.02f ms, %.0f files/s", d*1000, float64(len(files))/d)
	}

	err = checkDir(w.Dir)
	return
}

// CleanTempFiles removes all files that match the temporary filename pattern.
func (w *Walker) CleanTempFiles() {
	filepath.Walk(w.Dir, w.cleanTempFile)
}

func (w *Walker) lazyInit() {
	if w.suppressed == nil {
		w.suppressed = make(map[string]bool)
	}
}

func (w *Walker) loadIgnoreFiles(dir string, ign map[string][]string) filepath.WalkFunc {
	return func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		rn, err := filepath.Rel(dir, p)
		if err != nil {
			return nil
		}

		if pn, sn := filepath.Split(rn); sn == w.IgnoreFile {
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
		}

		return nil
	}
}

func (w *Walker) walkAndHashFiles(res *[]File, ign map[string][]string) filepath.WalkFunc {
	return func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if debug {
				dlog.Println("error:", p, info, err)
			}
			return nil
		}

		rn, err := filepath.Rel(w.Dir, p)
		if err != nil {
			if debug {
				dlog.Println("rel error:", p, err)
			}
			return nil
		}

		if rn == "." {
			return nil
		}

		if w.TempNamer != nil && w.TempNamer.IsTemporary(rn) {
			// A temporary file
			if debug {
				dlog.Println("temporary:", rn)
			}
			return nil
		}

		if _, sn := filepath.Split(rn); sn == w.IgnoreFile {
			// An ignore-file; these are ignored themselves
			if debug {
				dlog.Println("ignorefile:", rn)
			}
			return nil
		}

		if w.ignoreFile(ign, rn) {
			// An ignored file
			if debug {
				dlog.Println("ignored:", rn)
			}
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Mode().IsDir() {
			if w.CurrentFiler != nil {
				cf := w.CurrentFiler.CurrentFile(rn)
				if cf.Modified == info.ModTime().Unix() && cf.Flags == uint32(info.Mode()&os.ModePerm|protocol.FlagDirectory) {
					if debug {
						dlog.Println("unchanged:", cf)
					}
					*res = append(*res, cf)
				} else {
					f := File{
						Name:     rn,
						Version:  lamport.Default.Tick(0),
						Flags:    uint32(info.Mode()&os.ModePerm) | protocol.FlagDirectory,
						Modified: info.ModTime().Unix(),
					}
					if debug {
						dlog.Println("dir:", cf, f)
					}
					*res = append(*res, f)
				}
				return nil
			}
		}

		if info.Mode().IsRegular() {
			if w.CurrentFiler != nil {
				cf := w.CurrentFiler.CurrentFile(rn)
				if cf.Flags&protocol.FlagDeleted == 0 && cf.Modified == info.ModTime().Unix() {
					if debug {
						dlog.Println("unchanged:", cf)
					}
					*res = append(*res, cf)
					return nil
				}

				if w.Suppressor != nil && w.Suppressor.Suppress(rn, info) {
					if !w.suppressed[rn] {
						w.suppressed[rn] = true
						log.Printf("INFO: Changes to %q are being temporarily suppressed because it changes too frequently.", p)
						cf.Suppressed = true
						cf.Version++
					}
					if debug {
						dlog.Println("suppressed:", cf)
					}
					*res = append(*res, cf)
					return nil
				} else if w.suppressed[rn] {
					log.Printf("INFO: Changes to %q are no longer suppressed.", p)
					delete(w.suppressed, rn)
				}
			}

			fd, err := os.Open(p)
			if err != nil {
				if debug {
					dlog.Println("open:", p, err)
				}
				return nil
			}
			defer fd.Close()

			t0 := time.Now()
			blocks, err := Blocks(fd, w.BlockSize)
			if err != nil {
				if debug {
					dlog.Println("hash error:", rn, err)
				}
				return nil
			}
			if debug {
				t1 := time.Now()
				dlog.Println("hashed:", rn, ";", len(blocks), "blocks;", info.Size(), "bytes;", int(float64(info.Size())/1024/t1.Sub(t0).Seconds()), "KB/s")
			}
			f := File{
				Name:     rn,
				Version:  lamport.Default.Tick(0),
				Size:     info.Size(),
				Flags:    uint32(info.Mode()),
				Modified: info.ModTime().Unix(),
				Blocks:   blocks,
			}
			*res = append(*res, f)
		}

		return nil
	}
}

func (w *Walker) cleanTempFile(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeType == 0 && w.TempNamer.IsTemporary(path) {
		os.Remove(path)
	}
	return nil
}

func (w *Walker) ignoreFile(patterns map[string][]string, file string) bool {
	first, last := filepath.Split(file)
	for prefix, pats := range patterns {
		if len(prefix) == 0 || prefix == first || strings.HasPrefix(first, prefix+"/") {
			for _, pattern := range pats {
				if match, _ := filepath.Match(pattern, last); match {
					return true
				}
			}
		}
	}
	return false
}

func checkDir(dir string) error {
	if info, err := os.Stat(dir); err != nil {
		return err
	} else if !info.IsDir() {
		return errors.New(dir + ": not a directory")
	}
	return nil
}
