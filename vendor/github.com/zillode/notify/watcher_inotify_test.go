// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build linux

package notify

import (
	"os"
	"path/filepath"
	"testing"
)

func icreate(w *W, path string) WCase {
	cas := create(w, path)
	cas.Events = append(cas.Events,
		&Call{P: path, E: InCreate},
	)
	return cas
}

func iremove(w *W, path string) WCase {
	cas := remove(w, path)
	cas.Events = append(cas.Events,
		&Call{P: path, E: InDelete},
	)
	return cas
}

func iopen(w *W, path string) WCase {
	return WCase{
		Action: func() {
			f, err := os.OpenFile(filepath.Join(w.root, path), os.O_RDWR, 0644)
			if err != nil {
				w.Fatalf("OpenFile(%q)=%v", path, err)
			}
			if err := f.Close(); err != nil {
				w.Fatalf("Close(%q)=%v", path, err)
			}
		},
		Events: []EventInfo{
			&Call{P: path, E: InAccess},
			&Call{P: path, E: InOpen},
			&Call{P: path, E: InCloseNowrite},
		},
	}
}

func iread(w *W, path string, p []byte) WCase {
	return WCase{
		Action: func() {
			f, err := os.OpenFile(filepath.Join(w.root, path), os.O_RDWR, 0644)
			if err != nil {
				w.Fatalf("OpenFile(%q)=%v", path, err)
			}
			if _, err := f.Read(p); err != nil {
				w.Fatalf("Read(%q)=%v", path, err)
			}
			if err := f.Close(); err != nil {
				w.Fatalf("Close(%q)=%v", path, err)
			}
		},
		Events: []EventInfo{
			&Call{P: path, E: InAccess},
			&Call{P: path, E: InOpen},
			&Call{P: path, E: InModify},
			&Call{P: path, E: InCloseNowrite},
		},
	}
}

func iwrite(w *W, path string, p []byte) WCase {
	cas := write(w, path, p)
	path = cas.Events[0].Path()
	cas.Events = append(cas.Events,
		&Call{P: path, E: InAccess},
		&Call{P: path, E: InOpen},
		&Call{P: path, E: InModify},
		&Call{P: path, E: InCloseWrite},
	)
	return cas
}

func irename(w *W, path string) WCase {
	const ext = ".notify"
	return WCase{
		Action: func() {
			file := filepath.Join(w.root, path)
			if err := os.Rename(file, file+ext); err != nil {
				w.Fatalf("Rename(%q, %q)=%v", path, path+ext, err)
			}
		},
		Events: []EventInfo{
			&Call{P: path, E: InMovedFrom},
			&Call{P: path + ext, E: InMovedTo},
			&Call{P: path, E: InOpen},
			&Call{P: path, E: InAccess},
			&Call{P: path, E: InCreate},
		},
	}
}

var events = []Event{
	InAccess,
	InModify,
	InAttrib,
	InCloseWrite,
	InCloseNowrite,
	InOpen,
	InMovedFrom,
	InMovedTo,
	InCreate,
	InDelete,
	InDeleteSelf,
	InMoveSelf,
}

func TestWatcherInotify(t *testing.T) {
	w := NewWatcherTest(t, "testdata/vfs.txt", events...)
	defer w.Close()

	cases := [...]WCase{
		iopen(w, "src/github.com/rjeczalik/fs/fs.go"),
		iwrite(w, "src/github.com/rjeczalik/fs/fs.go", []byte("XD")),
		iread(w, "src/github.com/rjeczalik/fs/fs.go", []byte("XD")),
		iremove(w, "src/github.com/ppknap/link/README.md"),
		irename(w, "src/github.com/rjeczalik/fs/LICENSE"),
	}

	w.ExpectAny(cases[:])
}
