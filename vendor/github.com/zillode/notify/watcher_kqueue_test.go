// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin,kqueue dragonfly freebsd netbsd openbsd

package notify

import (
	"os"
	"path/filepath"
	"testing"
)

func kqremove(w *W, path string, files []string) WCase {
	cas := remove(w, path)
	cas.Events[0] = &Call{P: path, E: NoteDelete}
	for _, f := range files {
		cas.Events = append(cas.Events, &Call{P: f, E: NoteDelete})
	}
	return cas
}

func kqwrite(w *W, path string, p []byte) WCase {
	cas := write(w, path, p)
	path = cas.Events[0].Path()
	cas.Events[0] = &Call{P: path, E: NoteExtend | NoteWrite}
	return cas
}

func kqrename(w *W, path string, files []string) WCase {
	const ext = ".notify"
	cas := WCase{
		Action: func() {
			file := filepath.Join(w.root, path)
			if err := os.Rename(file, file+ext); err != nil {
				w.Fatalf("Rename(%q, %q)=%v", path, path+ext, err)
			}
		},
		Events: []EventInfo{
			&Call{P: path + ext, E: osSpecificCreate},
			&Call{P: path, E: NoteRename},
		},
	}
	for _, f := range files {
		cas.Events = append(cas.Events, &Call{P: f, E: NoteRename})
	}
	return cas
}

func kqlink(w *W, path string) WCase {
	const ext = ".notify"
	return WCase{
		Action: func() {
			file := filepath.Join(w.root, path)
			if err := os.Link(file, file+ext); err != nil {
				w.Fatalf("Link(%q, %q)=%v", path, path+ext, err)
			}
		},
		Events: []EventInfo{
			&Call{P: path, E: NoteLink},
			&Call{P: path + ext, E: osSpecificCreate},
		},
	}
}

var events = []Event{
	NoteWrite,
	NoteAttrib,
	NoteRename,
	osSpecificCreate,
	NoteDelete,
	NoteExtend,
	NoteLink,
}

func TestWatcherKqueue(t *testing.T) {
	w := NewWatcherTest(t, "testdata/vfs.txt", events...)
	defer w.Close()

	cases := [...]WCase{
		kqremove(w, "src/github.com/ppknap/link/include/coost/link", []string{
			"src/github.com/ppknap/link/include/coost/link/definitions.hpp",
			"src/github.com/ppknap/link/include/coost/link/detail/bundle.hpp",
			"src/github.com/ppknap/link/include/coost/link/detail/container_invoker.hpp",
			"src/github.com/ppknap/link/include/coost/link/detail/container_value_trait.hpp",
			"src/github.com/ppknap/link/include/coost/link/detail/dummy_type.hpp",
			"src/github.com/ppknap/link/include/coost/link/detail/function_trait.hpp",
			"src/github.com/ppknap/link/include/coost/link/detail/immediate_invoker.hpp",
			"src/github.com/ppknap/link/include/coost/link/detail/stdhelpers/always_same.hpp",
			"src/github.com/ppknap/link/include/coost/link/detail/stdhelpers/make_unique.hpp",
			"src/github.com/ppknap/link/include/coost/link/detail/stdhelpers",
			"src/github.com/ppknap/link/include/coost/link/detail/vertex.hpp",
			"src/github.com/ppknap/link/include/coost/link/detail/wire.hpp",
			"src/github.com/ppknap/link/include/coost/link/detail",
			"src/github.com/ppknap/link/include/coost/link/link.hpp",
		},
		),
		kqwrite(w, "src/github.com/rjeczalik/fs/fs.go", []byte("XD")),
		kqremove(w, "src/github.com/ppknap/link/README.md", nil),
		kqlink(w, "src/github.com/rjeczalik/fs/LICENSE"),
		kqrename(w, "src/github.com/rjeczalik/fs/fs.go", nil),
		kqrename(w, "src/github.com/rjeczalik/fs/cmd/gotree", []string{
			"src/github.com/rjeczalik/fs/cmd/gotree/go.go",
			"src/github.com/rjeczalik/fs/cmd/gotree/main.go",
		},
		),
	}

	w.ExpectAll(cases[:])
}
