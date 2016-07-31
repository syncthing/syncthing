// Copyright (C) 2016 The Protocol Authors.

package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"regexp"
	"testing"
)

var spaceRe = regexp.MustCompile(`\s`)

func TestVersion14Hello(t *testing.T) {
	// Tests that we can send and receive a version 0.14 hello message.

	expected := Hello{
		DeviceName:    "test device",
		ClientName:    "syncthing",
		ClientVersion: "v0.14.5",
	}
	msgBuf, err := expected.Marshal()
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

	send := &Hello{
		DeviceName:    "this device",
		ClientName:    "other client",
		ClientVersion: "v0.14.6",
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

func TestVersion13Hello(t *testing.T) {
	// Tests that we can send and receive a version 0.13 hello message.

	expected := Version13HelloMessage{
		DeviceName:    "test device",
		ClientName:    "syncthing",
		ClientVersion: "v0.13.5",
	}
	msgBuf := expected.MustMarshalXDR()

	hdrBuf := make([]byte, 8)
	binary.BigEndian.PutUint32(hdrBuf, Version13HelloMagic)
	binary.BigEndian.PutUint32(hdrBuf[4:], uint32(len(msgBuf)))

	outBuf := new(bytes.Buffer)
	outBuf.Write(hdrBuf)
	outBuf.Write(msgBuf)

	inBuf := new(bytes.Buffer)

	conn := &readWriter{outBuf, inBuf}

	send := Version13HelloMessage{
		DeviceName:    "this device",
		ClientName:    "other client",
		ClientVersion: "v0.13.6",
	}

	res, err := ExchangeHello(conn, send)
	if err != ErrTooOldVersion13 {
		t.Errorf("unexpected error %v != ErrTooOldVersion13", err)
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

func TestVersion12Hello(t *testing.T) {
	// Tests that we can correctly interpret the lack of a hello message
	// from a v0.12 client.

	// This is the typical v0.12 connection start - our message header for a
	// ClusterConfig message and then the cluster config message data. Taken
	// from a protocol dump of a recent v0.12 client.
	msg, _ := hex.DecodeString(spaceRe.ReplaceAllString(`
	00010001
	0000014a
	7802000070000000027332000100a00973796e637468696e670e00b000000876
	302e31322e32352400b00000000764656661756c741e00f01603000000204794
	03ffdef496b5f5e5bc9c0a15221e70073164509fa30761af63094f6f945c3800
	2073312f00f20b0001000000157463703a2f2f3132372e302e302e313a323230
	301f00012400080500003000001000f1122064516fb94d24e7b637d20d9846eb
	aeffb09556ef3968c8276fefc3fe24c144c2640002c0000034000f640002021f
	00004f00090400003000001100f11220dff67945f05bdab4270acd6057f1eacf
	a3ac93cade07ce6a89384c181ad6b80e640010332b000fc80007021f00012400
	080500046400041400f21f2dc2af5c5f28e38384295f2fc2af2052c3a46b736d
	c3b67267c3a57320e58aa8e4bd9c20d090d0b4d180d0b5d18136001f026c01b8
	90000000000000000000`, ``))

	outBuf := new(bytes.Buffer)
	outBuf.Write(msg)

	inBuf := new(bytes.Buffer)

	conn := &readWriter{outBuf, inBuf}

	send := Version13HelloMessage{
		DeviceName:    "this device",
		ClientName:    "other client",
		ClientVersion: "v0.13.6",
	}

	_, err := ExchangeHello(conn, send)
	if err != ErrTooOldVersion12 {
		t.Errorf("unexpected error %v != ErrTooOldVersion12", err)
	}
}

func TestUnknownHello(t *testing.T) {
	// Tests that we react correctly to a completely unknown magic number.

	// This is an unknown magic follow byte some message data.
	msg, _ := hex.DecodeString(spaceRe.ReplaceAllString(`
	12345678
	0000014a
	7802000070000000027332000100a00973796e637468696e670e00b000000876
	302e31322e32352400b00000000764656661756c741e00f01603000000204794
	03ffdef496b5f5e5bc9c0a15221e70073164509fa30761af63094f6f945c3800
	2073312f00f20b0001000000157463703a2f2f3132372e302e302e313a323230
	301f00012400080500003000001000f1122064516fb94d24e7b637d20d9846eb
	aeffb09556ef3968c8276fefc3fe24c144c2640002c0000034000f640002021f
	00004f00090400003000001100f11220dff67945f05bdab4270acd6057f1eacf
	a3ac93cade07ce6a89384c181ad6b80e640010332b000fc80007021f00012400
	080500046400041400f21f2dc2af5c5f28e38384295f2fc2af2052c3a46b736d
	c3b67267c3a57320e58aa8e4bd9c20d090d0b4d180d0b5d18136001f026c01b8
	90000000000000000000`, ``))

	outBuf := new(bytes.Buffer)
	outBuf.Write(msg)

	inBuf := new(bytes.Buffer)

	conn := &readWriter{outBuf, inBuf}

	send := Version13HelloMessage{
		DeviceName:    "this device",
		ClientName:    "other client",
		ClientVersion: "v0.13.6",
	}

	_, err := ExchangeHello(conn, send)
	if err != ErrUnknownMagic {
		t.Errorf("unexpected error %v != ErrUnknownMagic", err)
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
