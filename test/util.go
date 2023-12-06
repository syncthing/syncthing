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
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

type instance struct {
	deviceID     protocol.DeviceID
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
	cmd.Env = []string{"HOME=" + userHomeDir, "STNOUPGRADE=1"}
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
	env := []string{"HOME=" + inst.userHomeDir, "STNOUPGRADE=1", "STNORESTART=1", "STGUIADDRESS=127.0.0.1:0", "STGUIAPIKEY=" + inst.apiKey}

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
	return inst, nil
}

type listenAddressReader struct {
	log    *bytes.Buffer
	addrCh chan string
	idCh   chan protocol.DeviceID
}

func newListenAddressReader(r io.Reader) *listenAddressReader {
	sc := bufio.NewScanner(r)
	lr := &listenAddressReader{
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
