// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package netreflect implements run-time reflection for the
// facilities of net package.
package netreflect

import (
	"errors"
	"net"
)

var (
	errInvalidType = errors.New("invalid type")
	errOpNoSupport = errors.New("operation not supported")
)

// SocketOf returns the socket descriptor of c.
func SocketOf(c net.Conn) (uintptr, error) {
	switch c.(type) {
	case *net.TCPConn, *net.UDPConn, *net.IPConn, *net.UnixConn:
		return socketOf(c)
	default:
		return 0, errInvalidType
	}
}

// PacketSocketOf returns the socket descriptor of c.
func PacketSocketOf(c net.PacketConn) (uintptr, error) {
	switch c.(type) {
	case *net.UDPConn, *net.IPConn, *net.UnixConn:
		return socketOf(c.(net.Conn))
	default:
		return 0, errInvalidType
	}
}
