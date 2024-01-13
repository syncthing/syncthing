// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package result provides the result type for ignore matching. This is a
// separate package in order to break import cycles.
package ignoreresult

import "github.com/syncthing/syncthing/lib/build"

const (
	NotMatched R = 0

	// The bit flags are used by the ignore package to construct Results.
	// Don't use them for comparison; use the convenience methods.
	IgnoreBit R = 1 << iota
	DeletableBit
	FoldCaseBit
)

var Ignored = IgnoreBit

func init() {
	if build.IsDarwin || build.IsWindows {
		Ignored |= FoldCaseBit
	}
}

type R uint8

// IsIgnored returns true if the result is ignored.
func (r R) IsIgnored() bool {
	return r&IgnoreBit != 0
}

// IsDeletable returns true if the result is ignored and deletable.
func (r R) IsDeletable() bool {
	return r.IsIgnored() && r&DeletableBit != 0
}

// IsCaseFolded returns true if the result was a case-insensitive match.
func (r R) IsCaseFolded() bool {
	return r&FoldCaseBit != 0
}

// String returns a human readable representation of the result flags.
func (r R) String() string {
	var s string
	if r&IgnoreBit != 0 {
		s += "i"
	} else {
		s += "-"
	}
	if r&DeletableBit != 0 {
		s += "d"
	} else {
		s += "-"
	}
	if r&FoldCaseBit != 0 {
		s += "f"
	} else {
		s += "-"
	}
	return s
}
