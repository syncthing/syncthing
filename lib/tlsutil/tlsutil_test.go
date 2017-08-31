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
				// Check that we've nilled out the "first" thing
				if conn.(*UnionedConnection).first != nil {
					t.Errorf("%d: expected first read to clear out the `first` attribute", i)
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

type fakeAccepter struct {
	data []byte
}

func (f *fakeAccepter) Accept() (net.Conn, error) {
	return &fakeConn{f.data}, nil
}

func (f *fakeAccepter) Addr() net.Addr { return nil }
func (f *fakeAccepter) Close() error   { return nil }

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

func (f *fakeConn) Write(b []byte) (int, error) {
	return len(b), nil
}

func (f *fakeConn) Close() error                     { return nil }
func (f *fakeConn) LocalAddr() net.Addr              { return nil }
func (f *fakeConn) RemoteAddr() net.Addr             { return nil }
func (f *fakeConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(time.Time) error { return nil }
