// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"unicode"
)

func UnicodeLowercase(s string) string {
	rs := []rune(s)
	for i, r := range rs {
		rs[i] = unicode.ToLower(unicode.ToUpper(r))
	}
	return string(rs)
}
