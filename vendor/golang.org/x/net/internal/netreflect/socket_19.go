// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.9

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
		return 0, errOpNoSupport
	default:
		return 0, errInvalidType
	}
}

// PacketSocketOf returns the socket descriptor of c.
func PacketSocketOf(c net.PacketConn) (uintptr, error) {
	switch c.(type) {
	case *net.UDPConn, *net.IPConn, *net.UnixConn:
		return 0, errOpNoSupport
	default:
		return 0, errInvalidType
	}
}
