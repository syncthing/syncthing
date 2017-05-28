// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !go1.9
// +build darwin dragonfly freebsd linux netbsd openbsd solaris windows

package netreflect

import (
	"net"
	"reflect"
	"runtime"
)

func socketOf(c net.Conn) (uintptr, error) {
	v := reflect.ValueOf(c)
	switch e := v.Elem(); e.Kind() {
	case reflect.Struct:
		fd := e.FieldByName("conn").FieldByName("fd")
		switch e := fd.Elem(); e.Kind() {
		case reflect.Struct:
			sysfd := e.FieldByName("sysfd")
			if runtime.GOOS == "windows" {
				return uintptr(sysfd.Uint()), nil
			}
			return uintptr(sysfd.Int()), nil
		}
	}
	return 0, errInvalidType
}
