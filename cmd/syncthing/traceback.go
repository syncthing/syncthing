// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build go1.7
// +build go1.7

package toplevel

import "runtime/debug"

func init() {
	// We want all (our) goroutines in panic traces.
	debug.SetTraceback("all")
}
