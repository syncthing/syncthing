// Copyright (C) 2014 The Protocol Authors.

//go:build !windows && !darwin
// +build !windows,!darwin

package protocol

// Normal Unixes uses NFC and slashes, which is the wire format.

func makeNative(m contextLessModel) contextLessModel { return m }
