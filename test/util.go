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
	"net"
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
	apiAddress   string
	apiKey       string
	tcpPort      int
}

// startInstance starts a Syncthing instance with authentication. The
// username, password and API key are in the returned instance.
func startInstance(t *testing.T) *instance {
	t.Helper()

	// Use temporary directories for the Syncthing and user home
	// directories. The user home directory won't be used for anything, but
	// it needs to exist...
	syncthingDir := t.TempDir()
	userHomeDir := t.TempDir()

	inst := &instance{
		syncthingDir: syncthingDir,
		userHomeDir:  userHomeDir,
		apiKey:       rand.String(32),
	}

	// Start Syncthing with the config and API key.
	cmd := exec.Command(syncthingBinary, "--no-browser", "--no-default-folder", "--home", syncthingDir)
	cmd.Env = append(basicEnv(userHomeDir), "STGUIAPIKEY="+inst.apiKey)
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

	// Wait up to 30 seconds to get the device ID, which comes first.
	select {
	case inst.deviceID = <-lr.myIDCh:
	case <-time.After(30 * time.Second):
		t.Log(lr.log)
		t.Fatal("timeout waiting for device ID")
	}

	// Once we have that, the sync listeners & API should be up and running
	// quickly. Give it another few seconds.
	select {
	case inst.apiAddress = <-lr.apiAddrCh:
	case <-time.After(5 * time.Second):
		t.Log(lr.log)
		t.Fatal("timeout waiting for API address")
	}
	select {
	case inst.tcpPort = <-lr.tcpPortCh:
	case <-time.After(5 * time.Second):
		t.Log(lr.log)
		t.Fatal("timeout waiting for listen address")
	}

	return inst
}

func basicEnv(userHomeDir string) []string {
	return []string{"HOME=" + userHomeDir, "userprofile=" + userHomeDir, "STNOUPGRADE=1", "STNORESTART=1", "STMONITORED=1", "STGUIADDRESS=127.0.0.1:0"}
}

// syncthingMetadataReader reads the output of a Syncthing process and
// extracts the listen address and device ID. The results are in the channel
// fields, which can be read once.
type syncthingMetadataReader struct {
	log       *bytes.Buffer
	apiAddrCh chan string
	myIDCh    chan protocol.DeviceID
	tcpPortCh chan int
}

func newSyncthingMetadataReader(r io.Reader) *syncthingMetadataReader {
	sc := bufio.NewScanner(r)
	lr := &syncthingMetadataReader{
		log:       new(bytes.Buffer),
		apiAddrCh: make(chan string, 1),
		myIDCh:    make(chan protocol.DeviceID, 1),
		tcpPortCh: make(chan int, 1),
	}
	addrExp := regexp.MustCompile(`GUI and API listening on ([^\s]+)`)
	myIDExp := regexp.MustCompile(`My ID: ([^\s]+)`)
	tcpAddrExp := regexp.MustCompile(`TCP listener \((.+)\) starting`)
	go func() {
		for sc.Scan() {
			line := sc.Text()
			lr.log.WriteString(line + "\n")
			if m := addrExp.FindStringSubmatch(line); len(m) == 2 {
				lr.apiAddrCh <- m[1]
			}
			if m := myIDExp.FindStringSubmatch(line); len(m) == 2 {
				id, err := protocol.DeviceIDFromString(m[1])
				if err != nil {
					panic(err)
				}
				lr.myIDCh <- id
			}
			if m := tcpAddrExp.FindStringSubmatch(line); len(m) == 2 {
				addr, err := net.ResolveTCPAddr("tcp", m[1])
				if err != nil {
					panic(err)
				}
				lr.tcpPortCh <- addr.Port
			}
		}
	}()
	return lr
}

// generateTree generates n files with random data in a temporary directory
// and returns the path to the directory.
func generateTree(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		// Generate a random string. The first character is the directory
		// name, the rest is the file name.
		rnd := rand.String(16)
		sub := rnd[:1]
		file := rnd[1:]
		size := 512<<10 + rand.Intn(1024)<<10 // between 512 KiB and 1.5 MiB

		// Create the file with random data.
		os.Mkdir(filepath.Join(dir, sub), 0o700)
		lr := io.LimitReader(rand.Reader, int64(size))
		fd, err := os.Create(filepath.Join(dir, sub, file))
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

		if aInfo.Mode().IsRegular() {
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
