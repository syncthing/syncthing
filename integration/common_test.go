// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build integration

package integration_test

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	mr "math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type syncthingProcess struct {
	log  string
	argv []string
	port int

	cmd   *exec.Cmd
	logfd *os.File
}

func (p *syncthingProcess) start() error {
	if p.logfd == nil {
		logfd, err := os.Create(p.log)
		if err != nil {
			return err
		}
		p.logfd = logfd
	}

	cmd := exec.Command("../bin/syncthing", p.argv...)
	cmd.Stdout = p.logfd
	cmd.Stderr = p.logfd
	cmd.Env = append(env, fmt.Sprintf("STPROFILER=:%d", p.port+1000))

	err := cmd.Start()
	if err != nil {
		return err
	}
	p.cmd = cmd
	return nil
}

func (p *syncthingProcess) stop() {
	p.cmd.Process.Kill()
	p.cmd.Wait()
}

func (p *syncthingProcess) peerCompletion() (map[string]int, error) {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/rest/debug/peerCompletion", p.port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	comp := map[string]int{}
	err = json.NewDecoder(resp.Body).Decode(&comp)
	return comp, err
}

type fileGenerator struct {
	files   int
	maxexp  int
	srcname string
}

func generateFiles(dir string, files, maxexp int, srcname string) error {
	fd, err := os.Open(srcname)
	if err != nil {
		return err
	}

	for i := 0; i < files; i++ {
		n := randomName()
		p0 := filepath.Join(dir, string(n[0]), n[0:2])
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

		err = os.Chmod(p1, os.FileMode(mr.Intn(0777)|0400))
		if err != nil {
			return err
		}

		t := time.Now().Add(-time.Duration(mr.Intn(30*86400)) * time.Second)
		err = os.Chtimes(p1, t, t)
		if err != nil {
			return err
		}
	}

	return nil
}

func randomName() string {
	var b [16]byte
	rand.Reader.Read(b[:])
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
		err := os.RemoveAll(dir)
		if err != nil {
			return err
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
	abort := make(chan struct{})

	for i := range dirs {
		startWalker(dirs[i], chans[i], abort)
	}

	res := make([]fileInfo, len(dirs))
	for {
		numDone := 0
		for i := range chans {
			fi, ok := <-chans[i]
			if !ok {
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

type fileInfo struct {
	name string
	mode os.FileMode
	mod  time.Time
	hash [16]byte
}

func startWalker(dir string, res chan<- fileInfo, abort <-chan struct{}) {
	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rn, _ := filepath.Rel(dir, path)
		if rn == "." {
			return nil
		}

		var f fileInfo
		if info.IsDir() {
			f = fileInfo{
				name: rn,
				mode: info.Mode(),
				// hash and modtime zero for directories
			}
		} else {
			f = fileInfo{
				name: rn,
				mode: info.Mode(),
				mod:  info.ModTime(),
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
	go func() {
		filepath.Walk(dir, walker)
		close(res)
	}()
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
