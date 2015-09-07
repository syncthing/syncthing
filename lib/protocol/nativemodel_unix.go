// Copyright (C) 2014 The Protocol Authors.

// +build !windows,!darwin

package protocol

// Normal Unixes uses NFC and slashes, which is the wire format.

type nativeModel struct {
	Model
}
