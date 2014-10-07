// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// +build integration

package integration_test

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func init() {
	rand.Seed(42)
}

const (
	id1    = "I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU"
	id2    = "JMFJCXB-GZDE4BN-OCJE3VF-65GYZNU-AIVJRET-3J6HMRQ-AUQIGJO-FKNHMQU"
	apiKey = "abc123"
)

var env = []string{
	"HOME=.",
	"STTRACE=model",
	"STGUIAPIKEY=" + apiKey,
	"STNORESTART=1",
	"STPERFSTATS=1",
}

type syncthingProcess struct {
	log       string
	argv      []string
	port      int
	apiKey    string
	csrfToken string
	lastEvent int

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

	for {
		resp, err := p.get("/")
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (p *syncthingProcess) stop() {
	p.cmd.Process.Signal(os.Interrupt)
	p.cmd.Wait()
}

func (p *syncthingProcess) get(path string) (*http.Response, error) {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d%s", p.port, path), nil)
	if err != nil {
		return nil, err
	}
	if p.apiKey != "" {
		req.Header.Add("X-API-Key", p.apiKey)
	}
	if p.csrfToken != "" {
		req.Header.Add("X-CSRF-Token", p.csrfToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *syncthingProcess) peerCompletion() (map[string]int, error) {
	resp, err := p.get("/rest/debug/peerCompletion")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	comp := map[string]int{}
	err = json.NewDecoder(resp.Body).Decode(&comp)
	return comp, err
}

type event struct {
	ID   int
	Time time.Time
	Type string
	Data interface{}
}

func (p *syncthingProcess) events() ([]event, error) {
	resp, err := p.get(fmt.Sprintf("/rest/events?since=%d", p.lastEvent))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var evs []event
	err = json.NewDecoder(resp.Body).Decode(&evs)
	if err != nil {
		return nil, err
	}
	p.lastEvent = evs[len(evs)-1].ID
	return evs, err
}

type versionResp struct {
	Version string
}

func (p *syncthingProcess) version() (string, error) {
	resp, err := p.get("/rest/version")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var v versionResp
	err = json.NewDecoder(resp.Body).Decode(&v)
	if err != nil {
		return "", err
	}
	return v.Version, nil
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

		err = os.Chmod(p1, os.FileMode(rand.Intn(0777)|0400))
		if err != nil {
			return err
		}

		t := time.Now().Add(-time.Duration(rand.Intn(30*86400)) * time.Second)
		err = os.Chtimes(p1, t, t)
		if err != nil {
			return err
		}
	}

	return nil
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
		os.RemoveAll(dir)
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
	mod  int64
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
				mod:  info.ModTime().Unix(),
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
