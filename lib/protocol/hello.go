// Copyright (C) 2016 The Protocol Authors.

package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// The HelloIntf interface is implemented by the version specific hello
// message. It knows its magic number and how to serialize itself to a byte
// buffer.
type HelloIntf interface {
	Magic() uint32
	Marshal() ([]byte, error)
}

// The HelloResult is the non version specific interpretation of the other
// side's Hello message.
type HelloResult struct {
	DeviceName    string
	ClientName    string
	ClientVersion string
}

var (
	// ErrTooOldVersion12 is returned by ExchangeHello when the other side
	// speaks the older, incompatible version 0.12 of the protocol.
	ErrTooOldVersion12 = errors.New("the remote device speaks an older version of the protocol (v0.12) not compatible with this version")
	// ErrTooOldVersion13 is returned by ExchangeHello when the other side
	// speaks the older, incompatible version 0.12 of the protocol.
	ErrTooOldVersion13 = errors.New("the remote device speaks an older version of the protocol (v0.13) not compatible with this version")
	// ErrUnknownMagic is returned by ExchangeHellow when the other side
	// speaks something entirely unknown.
	ErrUnknownMagic = errors.New("the remote device speaks an unknown (newer?) version of the protocol")
)

func ExchangeHello(c io.ReadWriter, h HelloIntf) (HelloResult, error) {
	if err := writeHello(c, h); err != nil {
		return HelloResult{}, err
	}
	return readHello(c)
}

// IsVersionMismatch returns true if the error is a reliable indication of a
// version mismatch that we might want to alert the user about.
func IsVersionMismatch(err error) bool {
	switch err {
	case ErrTooOldVersion12, ErrTooOldVersion13, ErrUnknownMagic:
		return true
	default:
		return false
	}
}

func readHello(c io.Reader) (HelloResult, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(c, header); err != nil {
		return HelloResult{}, err
	}

	switch binary.BigEndian.Uint32(header) {
	case HelloMessageMagic:
		// This is a v0.14 Hello message in proto format
		if _, err := io.ReadFull(c, header[:2]); err != nil {
			return HelloResult{}, err
		}
		msgSize := binary.BigEndian.Uint16(header[:2])
		if msgSize > 32767 {
			return HelloResult{}, fmt.Errorf("hello message too big")
		}
		buf := make([]byte, msgSize)
		if _, err := io.ReadFull(c, buf); err != nil {
			return HelloResult{}, err
		}

		var hello Hello
		if err := hello.Unmarshal(buf); err != nil {
			return HelloResult{}, err
		}
		res := HelloResult{
			DeviceName:    hello.DeviceName,
			ClientName:    hello.ClientName,
			ClientVersion: hello.ClientVersion,
		}
		return res, nil

	case Version13HelloMagic:
		// This is a v0.13 Hello message in XDR format
		if _, err := io.ReadFull(c, header[:4]); err != nil {
			return HelloResult{}, err
		}
		msgSize := binary.BigEndian.Uint32(header[:4])
		if msgSize > 1024 {
			return HelloResult{}, fmt.Errorf("hello message too big")
		}
		buf := make([]byte, msgSize)
		if _, err := io.ReadFull(c, buf); err != nil {
			return HelloResult{}, err
		}

		var hello Version13HelloMessage
		if err := hello.UnmarshalXDR(buf); err != nil {
			return HelloResult{}, err
		}
		res := HelloResult(hello)
		return res, ErrTooOldVersion13

	case 0x00010001, 0x00010000:
		// This is the first word of a v0.12 cluster config message.
		// (Version 0, message ID 1, message type 0, compression enabled or disabled)
		return HelloResult{}, ErrTooOldVersion12
	}

	return HelloResult{}, ErrUnknownMagic
}

func writeHello(c io.Writer, h HelloIntf) error {
	msg, err := h.Marshal()
	if err != nil {
		return err
	}
	if len(msg) > 32767 {
		// The header length must be a positive signed int16
		panic("bug: attempting to serialize too large hello message")
	}

	header := make([]byte, 6)
	binary.BigEndian.PutUint32(header[:4], h.Magic())
	binary.BigEndian.PutUint16(header[4:], uint16(len(msg)))

	_, err = c.Write(append(header, msg...))
	return err
}
