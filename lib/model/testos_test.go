// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

// fatal is the required common interface between *testing.B and *testing.T
type fatal interface {
	Fatal(...interface{})
	Helper()
}

func must(f fatal, err error) {
	f.Helper()
	if err != nil {
		f.Fatal(err)
	}
}

func mustV[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
