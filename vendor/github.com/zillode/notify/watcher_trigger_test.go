// Copyright (c) 2014-2017 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin,kqueue dragonfly freebsd netbsd openbsd solaris

package notify

import "testing"

func TestWatcherCreateOnly(t *testing.T) {
	w := NewWatcherTest(t, "testdata/vfs.txt", Create)
	defer w.Close()

	cases := [...]WCase{
		create(w, "dir/"),
		create(w, "dir2/"),
	}

	w.ExpectAny(cases[:])
}
