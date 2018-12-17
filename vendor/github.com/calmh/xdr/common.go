// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

package xdr

import (
	"fmt"
	"reflect"
)

var padBytes = []byte{0, 0, 0}

// Pad returns the number of bytes that should be added to an item of length l
// bytes to conform to the XDR padding standard. This function is used by the
// generated marshalling code.
func Padding(l int) int {
	d := l % 4
	if d == 0 {
		return 0
	}
	return 4 - d
}

// ElementSizeExceeded returns an error describing the violated size
// constraint. This function is used by the generated marshalling code.
func ElementSizeExceeded(field string, size, limit int) error {
	return fmt.Errorf("%s exceeds size limit; %d > %d", field, size, limit)
}

type XDRSizer interface {
	XDRSize() int
}

// SizeOfSlice returns the XDR encoded size of the given []T. Supported types
// for T are string, []byte and types implementing XDRSizer. SizeOfSlice
// panics if the parameter is not a slice or if T is not one of the supported
// types. This function is used by the generated marshalling code.
func SizeOfSlice(ss interface{}) int {
	l := 0
	switch ss := ss.(type) {
	case []string:
		for _, s := range ss {
			l += 4 + len(s) + Padding(len(s))
		}

	case [][]byte:
		for _, s := range ss {
			l += 4 + len(s) + Padding(len(s))
		}

	default:
		v := reflect.ValueOf(ss)
		for i := 0; i < v.Len(); i++ {
			l += v.Index(i).Interface().(XDRSizer).XDRSize()
		}
	}

	return l
}
