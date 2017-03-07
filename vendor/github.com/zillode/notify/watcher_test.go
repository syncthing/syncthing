// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin linux freebsd dragonfly netbsd openbsd windows solaris

package notify

import "testing"

// NOTE Set DEBUG env var for extra debugging info.

func TestWatcher(t *testing.T) {
	w := NewWatcherTest(t, "testdata/vfs.txt")
	defer w.Close()

	cases := [...]WCase{
		create(w, "src/github.com/ppknap/link/include/coost/.link.hpp.swp"),
		create(w, "src/github.com/rjeczalik/fs/fs_test.go"),
		create(w, "src/github.com/rjeczalik/fs/binfs/"),
		create(w, "src/github.com/rjeczalik/fs/binfs.go"),
		create(w, "src/github.com/rjeczalik/fs/binfs_test.go"),
		remove(w, "src/github.com/rjeczalik/fs/binfs/"),
		create(w, "src/github.com/rjeczalik/fs/binfs/"),
		create(w, "src/github.com/rjeczalik/fs/virfs"),
		remove(w, "src/github.com/rjeczalik/fs/virfs"),
		create(w, "file"),
		create(w, "dir/"),
	}

	w.ExpectAny(cases[:])
}
