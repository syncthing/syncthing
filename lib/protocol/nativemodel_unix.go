// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows && !darwin
// +build !windows,!darwin

package protocol

// Normal Unixes uses NFC and slashes, which is the wire format.

func makeNative(m rawModel) rawModel { return m }
