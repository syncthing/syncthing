// Copyright (C) 2016 The Protocol Authors.

package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

var (
	// ErrTooOldVersion is returned by ExchangeHello when the other side
	// speaks an older, incompatible version of the protocol.
	ErrTooOldVersion = errors.New("the remote device speaks an older version of the protocol not compatible with this version")
	// ErrUnknownMagic is returned by ExchangeHello when the other side
	// speaks something entirely unknown.
	ErrUnknownMagic = errors.New("the remote device speaks an unknown (newer?) version of the protocol")
)

func ExchangeHello(c io.ReadWriter, h Hello) (Hello, error) {
	if h.Timestamp == 0 {
		panic("bug: missing timestamp in outgoing hello")
	}
	if err := writeHello(c, h); err != nil {
		return Hello{}, err
	}
	return readHello(c)
}

// IsVersionMismatch returns true if the error is a reliable indication of a
// version mismatch that we might want to alert the user about.
func IsVersionMismatch(err error) bool {
	switch err {
	case ErrTooOldVersion, ErrUnknownMagic:
		return true
	default:
		return false
	}
}

func readHello(c io.Reader) (Hello, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(c, header); err != nil {
		return Hello{}, err
	}

	switch binary.BigEndian.Uint32(header) {
	case HelloMessageMagic:
		// This is a v0.14 Hello message in proto format
		if _, err := io.ReadFull(c, header[:2]); err != nil {
			return Hello{}, err
		}
		msgSize := binary.BigEndian.Uint16(header[:2])
		if msgSize > 32767 {
			return Hello{}, errors.New("hello message too big")
		}
		buf := make([]byte, msgSize)
		if _, err := io.ReadFull(c, buf); err != nil {
			return Hello{}, err
		}

		var hello Hello
		if err := hello.Unmarshal(buf); err != nil {
			return Hello{}, err
		}
		return Hello(hello), nil

	case 0x00010001, 0x00010000, Version13HelloMagic:
		// This is the first word of an older cluster config message or an
		// old magic number. (Version 0, message ID 1, message type 0,
		// compression enabled or disabled)
		return Hello{}, ErrTooOldVersion
	}

	return Hello{}, ErrUnknownMagic
}

func writeHello(c io.Writer, h Hello) error {
	msg, err := h.Marshal()
	if err != nil {
		return err
	}
	if len(msg) > 32767 {
		// The header length must be a positive signed int16
		panic("bug: attempting to serialize too large hello message")
	}

	header := make([]byte, 6, 6+len(msg))
	binary.BigEndian.PutUint32(header[:4], h.Magic())
	binary.BigEndian.PutUint16(header[4:], uint16(len(msg)))

	_, err = c.Write(append(header, msg...))
	return err
}
