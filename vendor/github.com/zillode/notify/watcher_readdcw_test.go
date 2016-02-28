// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build windows

package notify

import "testing"

// TODO(ppknap) : remove notify.Create event.
func rcreate(w *W, path string) WCase {
	cas := create(w, path)
	cas.Events = append(cas.Events,
		&Call{P: path, E: FileActionAdded},
	)
	return cas
}

// TODO(ppknap) : remove notify.Remove event.
func rremove(w *W, path string) WCase {
	cas := remove(w, path)
	cas.Events = append(cas.Events,
		&Call{P: path, E: FileActionRemoved},
	)
	return cas
}

// TODO(ppknap) : remove notify.Rename event.
func rrename(w *W, oldpath, newpath string) WCase {
	cas := rename(w, oldpath, newpath)
	cas.Events = append(cas.Events,
		&Call{P: oldpath, E: FileActionRenamedOldName},
		&Call{P: newpath, E: FileActionRenamedNewName},
	)
	return cas
}

// TODO(ppknap) : remove notify.Write event.
func rwrite(w *W, path string, p []byte) WCase {
	cas := write(w, path, p)
	cas.Events = append(cas.Events,
		&Call{P: path, E: FileActionModified},
	)
	return cas
}

var events = []Event{
	FileNotifyChangeFileName,
	FileNotifyChangeDirName,
	FileNotifyChangeSize,
}

func TestWatcherReadDirectoryChangesW(t *testing.T) {
	w := NewWatcherTest(t, "testdata/vfs.txt", events...)
	defer w.Close()

	cases := [...]WCase{
		rcreate(w, "src/github.com/rjeczalik/fs/fs_windows.go"),
		rcreate(w, "src/github.com/rjeczalik/fs/subdir/"),
		rremove(w, "src/github.com/rjeczalik/fs/fs.go"),
		rrename(w, "src/github.com/rjeczalik/fs/LICENSE", "src/github.com/rjeczalik/fs/COPYLEFT"),
		rwrite(w, "src/github.com/rjeczalik/fs/cmd/gotree/go.go", []byte("XD")),
	}

	w.ExpectAny(cases[:])
}
