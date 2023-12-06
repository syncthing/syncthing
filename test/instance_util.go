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
	"net"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
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
