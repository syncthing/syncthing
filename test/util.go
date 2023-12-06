// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package integration

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"regexp"
	"testing"

	"github.com/syncthing/syncthing/lib/rand"
)

type instance struct {
	syncthingDir string
	userHomeDir  string
	address      string
	apiUser      string
	apiPassword  string
	apiKey       string
}

func startAuthenticatedInstance(t *testing.T) (*instance, error) {
	t.Helper()
	syncthingDir := t.TempDir()
	userHomeDir := t.TempDir()
	user := rand.String(8)
	password := rand.String(16)

	cmd := exec.Command("../bin/syncthing", "generate", "--home", syncthingDir, "--no-default-folder", "--skip-port-probing", "--gui-user", user, "--gui-password", password)
	cmd.Env = []string{"HOME=" + userHomeDir}
	buf := new(bytes.Buffer)
	cmd.Stdout = buf
	cmd.Stderr = buf
	if err := cmd.Run(); err != nil {
		t.Log(buf.String())
		return nil, err
	}

	inst, err := startInstanceInDir(t, syncthingDir, userHomeDir)
	if err != nil {
		return nil, err
	}

	inst.apiUser = user
	inst.apiPassword = password
	return inst, nil
}

func startUnauthenticatedInstance(t *testing.T) (*instance, error) {
	t.Helper()
	return startInstanceInDir(t, t.TempDir(), t.TempDir())
}

func startInstanceInDir(t *testing.T, syncthingDir, userHomeDir string) (*instance, error) {
	t.Helper()

	inst := &instance{
		syncthingDir: syncthingDir,
		userHomeDir:  userHomeDir,
		apiKey:       rand.String(32),
	}
	env := []string{"HOME=" + inst.userHomeDir, "STNORESTART=1", "STGUIADDRESS=127.0.0.1:0", "STGUIAPIKEY=" + inst.apiKey}

	cmd := exec.Command("../bin/syncthing", "--no-browser", "--home", syncthingDir)
	cmd.Env = env
	rd, wr := io.Pipe()
	cmd.Stdout = wr
	cmd.Stderr = wr
	lr := newListenAddressReader(rd)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	t.Cleanup(func() {
		cmd.Process.Signal(os.Interrupt)
		cmd.Wait()
	})

	inst.address = <-lr.addrCh
	return inst, nil
}

type listenAddressReader struct {
	addrCh chan string
}

func newListenAddressReader(r io.Reader) *listenAddressReader {
	sc := bufio.NewScanner(r)
	lr := &listenAddressReader{
		addrCh: make(chan string, 1),
	}
	exp := regexp.MustCompile(`GUI and API listening on ([^\s]+)`)
	go func() {
		for sc.Scan() {
			line := sc.Text()
			if m := exp.FindStringSubmatch(line); len(m) == 2 {
				lr.addrCh <- m[1]
			}
		}
	}()
	return lr
}
