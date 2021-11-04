// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// The existence of this file means we get 0% test coverage rather than no
// test coverage at all. Remove when implementing an actual test.

package tlsutil

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"
)

func TestUnionedConnection(t *testing.T) {
	cases := []struct {
		data  []byte
		isTLS bool
	}{
		{[]byte{0}, false},
		{[]byte{0x16}, true},
		{[]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0}, false},
		{[]byte{0x16, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0}, true},
	}

	for i, tc := range cases {
		fc := &fakeAccepter{tc.data}
		dl := DowngradingListener{fc, nil}

		conn, isTLS, err := dl.AcceptNoWrapTLS()
		if err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		if conn == nil {
			t.Fatalf("%d: unexpected nil conn", i)
		}
		if isTLS != tc.isTLS {
			t.Errorf("%d: isTLS=%v, expected %v", i, isTLS, tc.isTLS)
		}

		// Read all the data, check it's the same
		var bs []byte
		buf := make([]byte, 128)
		for {
			n, err := conn.Read(buf)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%d: read error: %v", i, err)
			}
			if len(bs) == 0 {
				// first read; should return just one byte
				if n != 1 {
					t.Errorf("%d: first read returned %d bytes, not 1", i, n)
				}
				if !conn.(*UnionedConnection).firstDone {
					t.Errorf("%d: expected first read to set the `firstDone` attribute", i)
				}
			}
			bs = append(bs, buf[:n]...)
		}
		if !bytes.Equal(bs, tc.data) {
			t.Errorf("%d: got wrong data", i)
		}

		t.Logf("%d: %v, %x", i, isTLS, bs)
	}
}

func TestCheckCipherSuites(t *testing.T) {
	// This is the set of cipher suites we expect - only the order should
	// differ.
	allSuites := []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	}

	suites := SecureDefaultWithTLS12().CipherSuites

	if len(suites) != len(allSuites) {
		t.Fatal("should get a list representing all suites")
	}

	// Check that the returned list of suites doesn't contain anything
	// unexpecteds and is free from duplicates.
	seen := make(map[uint16]struct{})
nextSuite:
	for _, s0 := range suites {
		if _, ok := seen[s0]; ok {
			t.Fatal("duplicate suite", s0)
		}
		for _, s1 := range allSuites {
			if s0 == s1 {
				seen[s0] = struct{}{}
				continue nextSuite
			}
		}
		t.Fatal("got unknown suite", s0)
	}
}

type fakeAccepter struct {
	data []byte
}

func (f *fakeAccepter) Accept() (net.Conn, error) {
	return &fakeConn{f.data}, nil
}

func (*fakeAccepter) Addr() net.Addr { return nil }
func (*fakeAccepter) Close() error   { return nil }

type fakeConn struct {
	data []byte
}

func (f *fakeConn) Read(b []byte) (int, error) {
	if len(f.data) == 0 {
		return 0, io.EOF
	}
	n := copy(b, f.data)
	f.data = f.data[n:]
	return n, nil
}

func (*fakeConn) Write(b []byte) (int, error) {
	return len(b), nil
}

func (*fakeConn) Close() error                     { return nil }
func (*fakeConn) LocalAddr() net.Addr              { return nil }
func (*fakeConn) RemoteAddr() net.Addr             { return nil }
func (*fakeConn) SetDeadline(time.Time) error      { return nil }
func (*fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (*fakeConn) SetWriteDeadline(time.Time) error { return nil }
