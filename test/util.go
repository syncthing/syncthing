// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"crypto/md5"
	cr "crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/rc"
	"github.com/syncthing/syncthing/lib/symlinks"
)

func init() {
	rand.Seed(42)
}

const (
	id1    = "I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU"
	id2    = "JMFJCXB-GZDE4BN-OCJE3VF-65GYZNU-AIVJRET-3J6HMRQ-AUQIGJO-FKNHMQU"
	id3    = "373HSRP-QLPNLIE-JYKZVQF-P4PKZ63-R2ZE6K3-YD442U2-JHBGBQG-WWXAHAU"
	apiKey = "abc123"
)

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
	src := io.LimitReader(&inifiteReader{fd}, int64(s))
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

	_ = os.Chmod(p1, os.FileMode(rand.Intn(0777)|0400))

	t := time.Now().Add(-time.Duration(rand.Intn(30*86400)) * time.Second)
	err = os.Chtimes(p1, t, t)
	if err != nil {
		return err
	}

	return nil
}

func alterFiles(dir string) error {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if os.IsNotExist(err) {
			// Something we deleted. Never mind.
			return nil
		}

		info, err = os.Stat(path)
		if err != nil {
			// Something we deleted while walking. Ignore.
			return nil
		}

		if strings.HasPrefix(filepath.Base(path), "test-") {
			return nil
		}

		switch filepath.Base(path) {
		case ".stfolder":
			return nil
		case ".stversions":
			return nil
		}

		// File structure is base/x/xy/xyz12345...
		// comps == 1: base (don't touch)
		// comps == 2: base/x (must be dir)
		// comps == 3: base/x/xy (must be dir)
		// comps  > 3: base/x/xy/xyz12345... (can be dir or file)

		comps := len(strings.Split(path, string(os.PathSeparator)))

		r := rand.Intn(10)
		switch {
		case r == 0 && comps > 2:
			// Delete every tenth file or directory, except top levels
			return removeAll(path)

		case r == 1 && info.Mode().IsRegular():
			if info.Mode()&0200 != 0200 {
				// Not owner writable. Fix.
				if err = os.Chmod(path, 0644); err != nil {
					return err
				}
			}

			// Overwrite a random kilobyte of every tenth file
			fd, err := os.OpenFile(path, os.O_RDWR, 0644)
			if err != nil {
				return err
			}
			if info.Size() > 1024 {
				_, err = fd.Seek(rand.Int63n(info.Size()), os.SEEK_SET)
				if err != nil {
					return err
				}
			}
			_, err = io.Copy(fd, io.LimitReader(cr.Reader, 1024))
			if err != nil {
				return err
			}
			return fd.Close()

		// Change capitalization
		case r == 2 && comps > 3 && rand.Float64() < 0.2:
			if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
				// Syncthing is currently broken for case-only renames on case-
				// insensitive platforms.
				// https://github.com/syncthing/syncthing/issues/1787
				return nil
			}

			base := []rune(filepath.Base(path))
			for i, r := range base {
				if rand.Float64() < 0.5 {
					base[i] = unicode.ToLower(r)
				} else {
					base[i] = unicode.ToUpper(r)
				}
			}
			newPath := filepath.Join(filepath.Dir(path), string(base))
			if newPath != path {
				return osutil.TryRename(path, newPath)
			}

		// Switch between files and directories
		case r == 3 && comps > 3 && rand.Float64() < 0.2:
			if !info.Mode().IsRegular() {
				err = removeAll(path)
				if err != nil {
					return err
				}
				d1 := []byte("I used to be a dir: " + path)
				err := ioutil.WriteFile(path, d1, 0644)
				if err != nil {
					return err
				}
			} else {
				err := osutil.Remove(path)
				if err != nil {
					return err
				}
				err = os.MkdirAll(path, 0755)
				if err != nil {
					return err
				}
				generateFiles(path, 10, 20, "../LICENSE")
			}
			return err

			/*
				This fails. Bug?

					// Rename the file, while potentially moving it up in the directory hiearachy
					case r == 4 && comps > 2 && (info.Mode().IsRegular() || rand.Float64() < 0.2):
						rpath := filepath.Dir(path)
						if rand.Float64() < 0.2 {
							for move := rand.Intn(comps - 1); move > 0; move-- {
								rpath = filepath.Join(rpath, "..")
							}
						}
						return osutil.TryRename(path, filepath.Join(rpath, randomName()))
			*/
		}

		return nil
	})
	if err != nil {
		return err
	}

	return generateFiles(dir, 25, 20, "../LICENSE")
}

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

func randomName() string {
	var b [16]byte
	ReadRand(b[:])
	return fmt.Sprintf("%x", b[:])
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

// rm -rf
func removeAll(dirs ...string) error {
	for _, dir := range dirs {
		files, err := osutil.Glob(dir)
		if err != nil {
			return err
		}
		for _, file := range files {
			// Set any non-writeable files and dirs to writeable. This is necessary for os.RemoveAll to work on Windows.
			filepath.Walk(file, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.Mode()&0700 != 0700 {
					os.Chmod(path, 0777)
				}
				return nil
			})
			os.RemoveAll(file)
		}
	}
	return nil
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
				return fmt.Errorf("Mismatch; %#v (%s) != %#v (%s)", res[i], dirs[i], res[0], dirs[0])
			}
		}

		if numDone == len(dirs) {
			return nil
		}
	}
}

func directoryContents(dir string) ([]fileInfo, error) {
	res := make(chan fileInfo)
	errc := startWalker(dir, res, nil)

	var files []fileInfo
	for f := range res {
		files = append(files, f)
	}

	return files, <-errc
}

func mergeDirectoryContents(c ...[]fileInfo) []fileInfo {
	m := make(map[string]fileInfo)

	for _, l := range c {
		for _, f := range l {
			if cur, ok := m[f.name]; !ok || cur.mod < f.mod {
				m[f.name] = f
			}
		}
	}

	res := make([]fileInfo, len(m))
	i := 0
	for _, f := range m {
		res[i] = f
		i++
	}

	sort.Sort(fileInfoList(res))
	return res
}

func compareDirectoryContents(actual, expected []fileInfo) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("len(actual) = %d; len(expected) = %d", len(actual), len(expected))
	}

	for i := range actual {
		if actual[i] != expected[i] {
			return fmt.Errorf("Mismatch; actual %#v != expected %#v", actual[i], expected[i])
		}
	}
	return nil
}

type fileInfo struct {
	name string
	mode os.FileMode
	mod  int64
	hash [16]byte
	size int64
}

func (f fileInfo) String() string {
	return fmt.Sprintf("%s %04o %d %x", f.name, f.mode, f.mod, f.hash)
}

type fileInfoList []fileInfo

func (l fileInfoList) Len() int {
	return len(l)
}

func (l fileInfoList) Less(a, b int) bool {
	return l[a].name < l[b].name
}

func (l fileInfoList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func startWalker(dir string, res chan<- fileInfo, abort <-chan struct{}) chan error {
	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rn, _ := filepath.Rel(dir, path)
		if rn == "." || rn == ".stfolder" {
			return nil
		}
		if rn == ".stversions" {
			return filepath.SkipDir
		}

		var f fileInfo
		if info.Mode()&os.ModeSymlink != 0 {
			f = fileInfo{
				name: rn,
				mode: os.ModeSymlink,
			}

			tgt, _, err := symlinks.Read(path)
			if err != nil {
				return err
			}
			h := md5.New()
			h.Write([]byte(tgt))
			hash := h.Sum(nil)

			copy(f.hash[:], hash)
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
				size: info.Size(),
			}
			sum, err := md5file(path)
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

func md5file(fname string) (hash [16]byte, err error) {
	f, err := os.Open(fname)
	if err != nil {
		return
	}
	defer f.Close()

	h := md5.New()
	io.Copy(h, f)
	hb := h.Sum(nil)
	copy(hash[:], hb)

	return
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "request canceled while waiting")
}

func getTestName() string {
	callers := make([]uintptr, 100)
	runtime.Callers(1, callers)
	for i, caller := range callers {
		f := runtime.FuncForPC(caller)
		if f != nil {
			if f.Name() == "testing.tRunner" {
				testf := runtime.FuncForPC(callers[i-1])
				if testf != nil {
					path := strings.Split(testf.Name(), ".")
					return path[len(path)-1]
				}
			}
		}
	}
	return time.Now().String()
}

func checkedStop(t *testing.T, p *rc.Process) {
	if _, err := p.Stop(); err != nil {
		t.Fatal(err)
	}
}

func startInstance(t *testing.T, i int) *rc.Process {
	log.Printf("Starting instance %d...", i)
	addr := fmt.Sprintf("127.0.0.1:%d", 8080+i)
	log := fmt.Sprintf("logs/%s-%d-%d.out", getTestName(), i, time.Now().Unix()%86400)

	p := rc.NewProcess(addr)
	p.LogTo(log)
	if err := p.Start("../bin/syncthing", "-home", fmt.Sprintf("h%d", i), "-no-browser"); err != nil {
		t.Fatal(err)
	}
	p.AwaitStartup()
	return p
}
