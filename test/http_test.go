// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !integration
// +build !integration

package integration

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestHTTP(t *testing.T) {
	t.Parallel()

	addr, err := startInstance(t)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("index", func(t *testing.T) {
		t.Parallel()

		// Check for explicit index.html

		res, err := http.Get(fmt.Sprintf("http://%s/index.html", addr))
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("Status %d != 200", res.StatusCode)
		}
		bs, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(bs) < 1024 {
			t.Errorf("Length %d < 1024", len(bs))
		}
		if !bytes.Contains(bs, []byte("</html>")) {
			t.Error("Incorrect response")
		}
		if res.Header.Get("Set-Cookie") == "" {
			t.Error("No set-cookie header")
		}
		res.Body.Close()

		// Check for implicit index.html

		res, err = http.Get(fmt.Sprintf("http://%s/", addr))
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("Status %d != 200", res.StatusCode)
		}
		bs, err = io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(bs) < 1024 {
			t.Errorf("Length %d < 1024", len(bs))
		}
		if !bytes.Contains(bs, []byte("</html>")) {
			t.Error("Incorrect response")
		}
		if res.Header.Get("Set-Cookie") == "" {
			t.Error("No set-cookie header")
		}
		res.Body.Close()
	})

	t.Run("options", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequest(http.MethodOptions, fmt.Sprintf("http://%s/rest/system/error/clear", addr), nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("Status %d != 204 for OPTIONS", res.StatusCode)
		}
	})

	t.Run("csrf", func(t *testing.T) {
		t.Parallel()

		// Should fail without CSRF

		req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/rest/system/error/clear", addr), nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("Status %d != 403 for POST", res.StatusCode)
		}

		// Get CSRF

		req, err = http.NewRequest("GET", fmt.Sprintf("http://%s/", addr), nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		hdr := res.Header.Get("Set-Cookie")
		id := res.Header.Get("X-Syncthing-ID")[:protocol.ShortIDStringLength]
		if !strings.Contains(hdr, "CSRF-Token") {
			t.Error("Missing CSRF-Token in", hdr)
		}

		// Should succeed with CSRF

		req, err = http.NewRequest("POST", fmt.Sprintf("http://%s/rest/system/error/clear", addr), nil)
		if err != nil {
			t.Fatal(err)
		}

		req.Header.Set("X-CSRF-Token-"+id, hdr[len("CSRF-Token-"+id+"="):])
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("Status %d != 200 for POST", res.StatusCode)
		}

		// Should fail with incorrect CSRF

		req, err = http.NewRequest("POST", fmt.Sprintf("http://%s/rest/system/error/clear", addr), nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-CSRF-Token-"+id, hdr[len("CSRF-Token-"+id+"="):]+"X")
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("Status %d != 403 for POST", res.StatusCode)
		}
	})
}

func startInstance(t *testing.T) (string, error) {
	cmd := exec.Command("../bin/syncthing", "--no-browser", "--home", t.TempDir())
	cmd.Env = append(os.Environ(), "STNORESTART=1", "STGUIADDRESS=127.0.0.1:0")
	rd, wr := io.Pipe()
	cmd.Stdout = wr
	cmd.Stderr = wr
	lr := newListenAddressReader(rd)

	if err := cmd.Start(); err != nil {
		return "", err
	}

	t.Cleanup(func() {
		cmd.Process.Signal(os.Interrupt)
		cmd.Wait()
	})

	return <-lr.addrCh, nil
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
