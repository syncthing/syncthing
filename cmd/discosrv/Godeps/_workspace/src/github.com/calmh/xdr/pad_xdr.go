// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

// +build !ipdr

package xdr

func pad(l int) int {
	d := l % 4
	if d == 0 {
		return 0
	}
	return 4 - d
}
