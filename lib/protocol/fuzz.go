// Copyright (C) 2015 The Protocol Authors.

// +build gofuzz

package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"reflect"
	"sync"
)

func Fuzz(data []byte) int {
	// Regenerate the length, or we'll most commonly exit quickly due to an
	// unexpected eof which is unintestering.
	if len(data) > 8 {
		binary.BigEndian.PutUint32(data[4:], uint32(len(data))-8)
	}

	// Setup a rawConnection we'll use to parse the message.
	c := rawConnection{
		cr:     &countingReader{Reader: bytes.NewReader(data)},
		closed: make(chan struct{}),
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, BlockSize)
			},
		},
	}

	// Attempt to parse the message.
	hdr, msg, err := c.readMessage()
	if err != nil {
		return 0
	}

	// If parsing worked, attempt to encode it again.
	newBs, err := msg.AppendXDR(nil)
	if err != nil {
		panic("not encodable")
	}

	// Create an appriate header for the re-encoding.
	newMsg := make([]byte, 8)
	binary.BigEndian.PutUint32(newMsg, encodeHeader(hdr))
	binary.BigEndian.PutUint32(newMsg[4:], uint32(len(newBs)))
	newMsg = append(newMsg, newBs...)

	// Use the rawConnection to parse the re-encoding.
	c.cr = &countingReader{Reader: bytes.NewReader(newMsg)}
	hdr2, msg2, err := c.readMessage()
	if err != nil {
		fmt.Println("Initial:\n" + hex.Dump(data))
		fmt.Println("New:\n" + hex.Dump(newMsg))
		panic("not parseable after re-encode: " + err.Error())
	}

	// Make sure the data is the same as it was before.
	if hdr != hdr2 {
		panic("headers differ")
	}
	if !reflect.DeepEqual(msg, msg2) {
		panic("contents differ")
	}

	return 1
}
