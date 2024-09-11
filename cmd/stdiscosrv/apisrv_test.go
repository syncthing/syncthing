// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"
)

func TestFixupAddresses(t *testing.T) {
	cases := []struct {
		remote *net.TCPAddr
		in     []string
		out    []string
	}{
		{ // verbatim passthrough
			in:  []string{"tcp://1.2.3.4:22000"},
			out: []string{"tcp://1.2.3.4:22000"},
		}, { // unspecified replaced by remote
			remote: addr("1.2.3.4", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://1.2.3.4:22000", "tcp://192.0.2.42:22000"},
		}, { // unspecified not used as replacement
			remote: addr("0.0.0.0", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // unspecified not used as replacement
			remote: addr("::", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // localhost not used as replacement
			remote: addr("127.0.0.1", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // localhost not used as replacement
			remote: addr("::1", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // multicast not used as replacement
			remote: addr("224.0.0.1", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // multicast not used as replacement
			remote: addr("ff80::42", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // explicitly announced weirdness is also filtered
			remote: addr("192.0.2.42", 22000),
			in:     []string{"tcp://:22000", "tcp://127.1.2.3:22000", "tcp://[::1]:22000", "tcp://[ff80::42]:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // port remapping
			remote: addr("123.123.123.123", 9000),
			in:     []string{"tcp://0.0.0.0:0"},
			out:    []string{"tcp://123.123.123.123:9000"},
		}, { // unspecified port remapping
			remote: addr("123.123.123.123", 9000),
			in:     []string{"tcp://:0"},
			out:    []string{"tcp://123.123.123.123:9000"},
		}, { // empty remapping
			remote: addr("123.123.123.123", 9000),
			in:     []string{"tcp://"},
			out:    []string{},
		}, { // port only remapping
			remote: addr("123.123.123.123", 9000),
			in:     []string{"tcp://44.44.44.44:0"},
			out:    []string{"tcp://44.44.44.44:9000"},
		}, { // remote ip nil
			remote: addr("", 9000),
			in:     []string{"tcp://:22000", "tcp://44.44.44.44:9000"},
			out:    []string{"tcp://44.44.44.44:9000"},
		}, { // remote port 0
			remote: addr("123.123.123.123", 0),
			in:     []string{"tcp://:22000", "tcp://44.44.44.44"},
			out:    []string{"tcp://123.123.123.123:22000"},
		},
	}

	for _, tc := range cases {
		out := fixupAddresses(tc.remote, tc.in)
		if fmt.Sprint(out) != fmt.Sprint(tc.out) {
			t.Errorf("fixupAddresses(%v, %v) => %v, expected %v", tc.remote, tc.in, out, tc.out)
		}
	}
}

func addr(host string, port int) *net.TCPAddr {
	return &net.TCPAddr{
		IP:   net.ParseIP(host),
		Port: port,
	}
}

func BenchmarkAPIRequests(b *testing.B) {
	db := newInMemoryStore(b.TempDir(), 0, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go db.Serve(ctx)
	api := newAPISrv("127.0.0.1:0", tls.Certificate{}, db, nil, true, true)
	srv := httptest.NewServer(http.HandlerFunc(api.handler))

	kf := b.TempDir() + "/cert"
	crt, err := tlsutil.NewCertificate(kf+".crt", kf+".key", "localhost", 7)
	if err != nil {
		b.Fatal(err)
	}
	certBs, err := os.ReadFile(kf + ".crt")
	if err != nil {
		b.Fatal(err)
	}
	certBs = regexp.MustCompile(`---[^\n]+---\n`).ReplaceAll(certBs, nil)
	certString := string(strings.ReplaceAll(string(certBs), "\n", " "))

	devID := protocol.NewDeviceID(crt.Certificate[0])
	devIDString := devID.String()

	b.Run("Announce", func(b *testing.B) {
		b.ReportAllocs()
		url := srv.URL + "/v2/?device=" + devIDString
		for i := 0; i < b.N; i++ {
			req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"addresses":["tcp://10.10.10.10:42000"]}`))
			req.Header.Set("X-Forwarded-Tls-Client-Cert", certString)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				b.Fatalf("unexpected status %s", resp.Status)
			}
		}
	})

	b.Run("Lookup", func(b *testing.B) {
		b.ReportAllocs()
		url := srv.URL + "/v2/?device=" + devIDString
		for i := 0; i < b.N; i++ {
			req, _ := http.NewRequest(http.MethodGet, url, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				b.Fatal(err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				b.Fatalf("unexpected status %s", resp.Status)
			}
		}
	})

	b.Run("LookupNoCompression", func(b *testing.B) {
		b.ReportAllocs()
		url := srv.URL + "/v2/?device=" + devIDString
		for i := 0; i < b.N; i++ {
			req, _ := http.NewRequest(http.MethodGet, url, nil)
			req.Header.Set("Accept-Encoding", "identity") // disable compression
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				b.Fatal(err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				b.Fatalf("unexpected status %s", resp.Status)
			}
		}
	})
}
