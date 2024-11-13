// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestVersion14Hello(t *testing.T) {
	// Tests that we can send and receive a version 0.14 hello message.

	expected := Hello{
		DeviceName:    "test device",
		ClientName:    "syncthing",
		ClientVersion: "v0.14.5",
	}
	msgBuf, err := proto.Marshal(expected.toWire())
	if err != nil {
		t.Fatal(err)
	}

	hdrBuf := make([]byte, 6)
	binary.BigEndian.PutUint32(hdrBuf, HelloMessageMagic)
	binary.BigEndian.PutUint16(hdrBuf[4:], uint16(len(msgBuf)))

	outBuf := new(bytes.Buffer)
	outBuf.Write(hdrBuf)
	outBuf.Write(msgBuf)

	inBuf := new(bytes.Buffer)

	conn := &readWriter{outBuf, inBuf}

	send := Hello{
		DeviceName:    "this device",
		ClientName:    "other client",
		ClientVersion: "v0.14.6",
		Timestamp:     1234567890,
	}

	res, err := ExchangeHello(conn, send)
	if err != nil {
		t.Fatal(err)
	}

	if res.ClientName != expected.ClientName {
		t.Errorf("incorrect ClientName %q != expected %q", res.ClientName, expected.ClientName)
	}
	if res.ClientVersion != expected.ClientVersion {
		t.Errorf("incorrect ClientVersion %q != expected %q", res.ClientVersion, expected.ClientVersion)
	}
	if res.DeviceName != expected.DeviceName {
		t.Errorf("incorrect DeviceName %q != expected %q", res.DeviceName, expected.DeviceName)
	}
}

func TestOldHelloMsgs(t *testing.T) {
	// Tests that we can correctly identify old/missing/unknown hello
	// messages.

	cases := []struct {
		msg string
		err error
	}{
		{"00010001", ErrTooOldVersion}, // v12
		{"9F79BC40", ErrTooOldVersion}, // v13
		{"12345678", ErrUnknownMagic},
	}

	for _, tc := range cases {
		msg, _ := hex.DecodeString(tc.msg)

		outBuf := new(bytes.Buffer)
		outBuf.Write(msg)

		inBuf := new(bytes.Buffer)

		conn := &readWriter{outBuf, inBuf}

		send := Hello{
			DeviceName:    "this device",
			ClientName:    "other client",
			ClientVersion: "v1.0.0",
			Timestamp:     1234567890,
		}

		_, err := ExchangeHello(conn, send)
		if err != tc.err {
			t.Errorf("unexpected error %v != %v", err, tc.err)
		}
	}
}

type readWriter struct {
	r io.Reader
	w io.Writer
}

func (rw *readWriter) Write(data []byte) (int, error) {
	return rw.w.Write(data)
}

func (rw *readWriter) Read(data []byte) (int, error) {
	return rw.r.Read(data)
}
