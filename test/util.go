// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package integration

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"golang.org/x/exp/slices"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sha256"
)

const syncthingBinary = "../bin/syncthing"

// instance represents a running instance of Syncthing.
type instance struct {
	deviceID     protocol.DeviceID
	syncthingDir string
	userHomeDir  string
	address      string
	apiUser      string
	apiPassword  string
	apiKey       string
}

// startAuthenticatedInstance starts a Syncthing instance with
// authentication. The username, password and API key are in the returned
// instance.
func startAuthenticatedInstance(t *testing.T) *instance {
	t.Helper()
	syncthingDir := t.TempDir()
	userHomeDir := t.TempDir()
	user := rand.String(8)
	password := rand.String(16)

	cmd := exec.Command(syncthingBinary, "generate", "--home", syncthingDir, "--no-default-folder", "--skip-port-probing", "--gui-user", user, "--gui-password", password)
	cmd.Env = basicEnv(userHomeDir)
	buf := new(bytes.Buffer)
	cmd.Stdout = buf
	cmd.Stderr = buf
	if err := cmd.Run(); err != nil {
		t.Log(buf.String())
		t.Fatal(err)
	}

	inst := startInstanceInDir(t, syncthingDir, userHomeDir)
	inst.apiUser = user
	inst.apiPassword = password
	return inst
}

// startUnauthenticatedInstance starts a Syncthing instance without
// authentication.
func startUnauthenticatedInstance(t *testing.T) *instance {
	t.Helper()
	return startInstanceInDir(t, t.TempDir(), t.TempDir())
}

// startInstanceInDir starts a Syncthing instance in the given directory.
func startInstanceInDir(t *testing.T, syncthingDir, userHomeDir string) *instance {
	t.Helper()

	inst := &instance{
		syncthingDir: syncthingDir,
		userHomeDir:  userHomeDir,
		apiKey:       rand.String(32),
	}
	env := append(basicEnv(userHomeDir), "STGUIAPIKEY="+inst.apiKey)

	cmd := exec.Command(syncthingBinary, "--no-browser", "--home", syncthingDir)
	cmd.Env = env
	rd, wr := io.Pipe()
	cmd.Stdout = wr
	cmd.Stderr = wr
	lr := newSyncthingMetadataReader(rd)

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	// Wait up to 15 seconds to get the device ID, which comes first.
	select {
	case inst.deviceID = <-lr.idCh:
	case <-time.After(15 * time.Second):
		t.Log(lr.log)
		t.Fatal("timeout waiting for device ID")
	}
	// Once we have that, the API should be up and running quickly. Give it
	// another few seconds.
	select {
	case inst.address = <-lr.addrCh:
	case <-time.After(5 * time.Second):
		t.Log(lr.log)
		t.Fatal("timeout waiting for listen address")
	}
	return inst
}

func basicEnv(userHomeDir string) []string {
	return []string{"HOME=" + userHomeDir, "userprofile=" + userHomeDir, "STNOUPGRADE=1", "STNORESTART=1", "STMONITORED=1", "STGUIADDRESS=127.0.0.1:0"}
}

// Generates n files with random data in a temporary directory and returns
// the path to the directory.
func generateFiles(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		f := filepath.Join(dir, rand.String(8))
		size := 512<<10 + rand.Intn(1024)<<10 // between 512 KiB and 1.5 MiB
		lr := io.LimitReader(rand.Reader, int64(size))
		fd, err := os.Create(f)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(fd, lr)
		if err != nil {
			t.Fatal(err)
		}
		if err := fd.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// syncthingMetadataReader reads the output of a Syncthing process and
// extracts the listen address and device ID. The results are in the channel
// fields, which can be read once.
type syncthingMetadataReader struct {
	log    *bytes.Buffer
	addrCh chan string
	idCh   chan protocol.DeviceID
}

func newSyncthingMetadataReader(r io.Reader) *syncthingMetadataReader {
	sc := bufio.NewScanner(r)
	lr := &syncthingMetadataReader{
		log:    new(bytes.Buffer),
		addrCh: make(chan string, 1),
		idCh:   make(chan protocol.DeviceID, 1),
	}
	addrExp := regexp.MustCompile(`GUI and API listening on ([^\s]+)`)
	myIDExp := regexp.MustCompile(`My ID: ([^\s]+)`)
	go func() {
		for sc.Scan() {
			line := sc.Text()
			lr.log.WriteString(line + "\n")
			if m := addrExp.FindStringSubmatch(line); len(m) == 2 {
				lr.addrCh <- m[1]
			}
			if m := myIDExp.FindStringSubmatch(line); len(m) == 2 {
				id, err := protocol.DeviceIDFromString(m[1])
				if err != nil {
					panic(err)
				}
				lr.idCh <- id
			}
		}
	}()
	return lr
}

// compareTrees compares the contents of two directories recursively. It
// reports any differences as test failures.
func compareTrees(t *testing.T, a, b string) {
	t.Helper()

	// These will not match, so we ignore them.
	ignore := []string{".", ".stfolder"}

	if err := filepath.Walk(a, func(path string, aInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(a, path)
		if err != nil {
			return err
		}

		if slices.Contains(ignore, rel) {
			return nil
		}

		bPath := filepath.Join(b, rel)
		bInfo, err := os.Stat(bPath)
		if err != nil {
			return err
		}

		if aInfo.IsDir() != bInfo.IsDir() {
			t.Errorf("mismatched directory/file: %q", rel)
		}

		if aInfo.Mode() != bInfo.Mode() {
			t.Errorf("mismatched mode: %q", rel)
		}

		if !aInfo.ModTime().Equal(bInfo.ModTime()) {
			t.Errorf("mismatched mod time: %q", rel)
		}

		if aInfo.Size() != bInfo.Size() {
			t.Errorf("mismatched size: %q", rel)
		}

		aHash, err := sha256file(path)
		if err != nil {
			return err
		}
		bHash, err := sha256file(bPath)
		if err != nil {
			return err
		}
		if aHash != bHash {
			t.Errorf("mismatched hash: %q", rel)
		}

		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func sha256file(fname string) (string, error) {
	f, err := os.Open(fname)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	hb := h.Sum(nil)
	return fmt.Sprintf("%x", hb), nil
}
