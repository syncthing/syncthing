// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

import "testing"

func TestRecursiveTree(t *testing.T) {
	n := NewRecursiveTreeTest(t, "testdata/vfs.txt")
	defer n.Close()

	ch := NewChans(5)

	watches := [...]RCase{
		// i=0
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs/fs.go",
				C: ch[0],
				E: Create,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/rjeczalik/fs/fs.go",
					E: Create,
				},
			},
		},
		// i=1
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs/cmd/...",
				C: ch[1],
				E: Remove,
			},
			Record: []Call{
				{
					F: FuncRecursiveWatch,
					P: "src/github.com/rjeczalik/fs/cmd",
					E: Remove,
				},
			},
		},
		// i=2
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs",
				C: ch[2],
				E: Rename,
			},
			Record: []Call{
				{
					F: FuncRecursiveWatch,
					P: "src/github.com/rjeczalik/fs",
					E: Create | Remove | Rename,
				},
				{
					F: FuncRecursiveUnwatch,
					P: "src/github.com/rjeczalik/fs/cmd",
				},
				{
					F: FuncUnwatch,
					P: "src/github.com/rjeczalik/fs/fs.go",
				},
			},
		},
		// i=3
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/ppknap/link/README.md",
				C: ch[0],
				E: Create,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/ppknap/link/README.md",
					E: Create,
				},
			},
		},
		// i=4
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/ppknap/link/include/...",
				C: ch[3],
				E: Remove,
			},
			Record: []Call{
				{
					F: FuncRecursiveWatch,
					P: "src/github.com/ppknap/link/include",
					E: Remove,
				},
			},
		},
		// i=5
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/ppknap/link/include",
				C: ch[0],
				E: Write,
			},
			Record: []Call{
				{
					F:  FuncRecursiveRewatch,
					P:  "src/github.com/ppknap/link/include",
					NP: "src/github.com/ppknap/link/include",
					E:  Remove,
					NE: Remove | Write,
				},
			},
		},
		// i=6
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/ppknap/link/test/Jamfile.jam",
				C: ch[0],
				E: Rename,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/ppknap/link/test/Jamfile.jam",
					E: Rename,
				},
			},
		},
		// i=7
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/ppknap/link/test/Jamfile.jam",
				C: ch[0],
				E: Create,
			},
			Record: []Call{
				{
					F:  FuncRewatch,
					P:  "src/github.com/ppknap/link/test/Jamfile.jam",
					E:  Rename,
					NE: Rename | Create,
				},
			},
		},
		// i=8
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/ppknap/...",
				C: ch[0],
				E: Create,
			},
			Record: []Call{
				{
					F: FuncRecursiveWatch,
					P: "src/github.com/ppknap",
					E: Create | Remove | Write | Rename,
				},
				{
					F: FuncUnwatch,
					P: "src/github.com/ppknap/link/README.md",
				},
				{
					F: FuncRecursiveUnwatch,
					P: "src/github.com/ppknap/link/include",
				},
				{
					F: FuncUnwatch,
					P: "src/github.com/ppknap/link/test/Jamfile.jam",
				},
			},
		},
		// i=9
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs/README.md",
				C: ch[0],
				E: Rename,
			},
			Record: nil,
		},
		// i=10
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs/cmd/gotree",
				C: ch[2],
				E: Create | Remove,
			},
			Record: nil,
		},
		// i=11
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/pblaszczyk/qttu/src/main.cc",
				C: ch[0],
				E: Create,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/pblaszczyk/qttu/src/main.cc",
					E: Create,
				},
			},
		},
		// i=12
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/pblaszczyk/qttu/src/main.cc",
				C: ch[0],
				E: Remove,
			},
			Record: []Call{
				{
					F:  FuncRewatch,
					P:  "src/github.com/pblaszczyk/qttu/src/main.cc",
					E:  Create,
					NE: Create | Remove,
				},
			},
		},
		// i=13
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/pblaszczyk/qttu/src/main.cc",
				C: ch[0],
				E: Create | Remove,
			},
			Record: nil,
		},
		// i=14
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/pblaszczyk/qttu/src",
				C: ch[0],
				E: Create,
			},
			Record: []Call{
				{
					F:  FuncRecursiveRewatch,
					P:  "src/github.com/pblaszczyk/qttu/src/main.cc",
					NP: "src/github.com/pblaszczyk/qttu/src",
					E:  Create | Remove,
					NE: Create | Remove,
				},
			},
		},
		// i=15
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/pblaszczyk/qttu",
				C: ch[4],
				E: Write,
			},
			Record: []Call{
				{
					F:  FuncRecursiveRewatch,
					P:  "src/github.com/pblaszczyk/qttu/src",
					NP: "src/github.com/pblaszczyk/qttu",
					E:  Create | Remove,
					NE: Create | Remove | Write,
				},
			},
		},
		// i=16
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs/fs.go",
				C: ch[3],
				E: Rename,
			},
			Record: nil,
		},
	}

	n.ExpectRecordedCalls(watches[:])

	events := [...]TCase{
		// i=0
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/fs.go", E: Rename},
			Receiver: Chans{ch[2], ch[3]},
		},
		// i=1
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/fs.go", E: Create},
			Receiver: Chans{ch[0]},
		},
		// i=2
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/fs.go/file", E: Create},
			Receiver: Chans{ch[0]},
		},
		// i=3
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs", E: Rename},
			Receiver: Chans{ch[2]},
		},
		// i=4
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/fs_test.go", E: Rename},
			Receiver: Chans{ch[2]},
		},
		// i=5
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/mktree/main.go", E: Remove},
			Receiver: Chans{ch[1]},
		},
		// i=6
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/gotree", E: Remove},
			Receiver: Chans{ch[1], ch[2]},
		},
		// i=7
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd", E: Remove},
			Receiver: Chans{ch[1]},
		},
		// i=8
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/fs.go/file", E: Write},
			Receiver: nil,
		},
		// i=9
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/fs.go/file", E: Write},
			Receiver: nil,
		},
		// i=10
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs", E: Remove},
			Receiver: nil,
		},
		// i=11
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd", E: Rename},
			Receiver: Chans{ch[2]},
		},
		// i=12
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/mktree/main.go", E: Write},
			Receiver: nil,
		},
		// i=13
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/gotree", E: Rename},
			Receiver: nil,
		},
		// i=14
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/file", E: Rename},
			Receiver: nil,
		},
		// i=15
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/fs.go", E: Rename},
			Receiver: Chans{ch[2], ch[3]},
		},
	}

	n.ExpectTreeEvents(events[:], ch)

	stops := [...]RCase{
		// i=0
		{
			Call: Call{
				F: FuncStop,
				C: ch[1],
			},
			Record: nil,
		},
		{
			Call: Call{
				F: FuncStop,
				C: ch[4],
			},
			Record: []Call{
				{
					F:  FuncRecursiveRewatch,
					P:  "src/github.com/pblaszczyk/qttu",
					NP: "src/github.com/pblaszczyk/qttu",
					E:  Create | Remove | Write,
					NE: Create | Remove,
				},
			},
		},
	}

	n.ExpectRecordedCalls(stops[:])
}

func TestRecursiveTreeWatchInactiveMerge(t *testing.T) {
	n := NewRecursiveTreeTest(t, "testdata/vfs.txt")
	defer n.Close()

	ch := NewChans(1)

	watches := [...]RCase{
		// i=0
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs",
				C: ch[0],
				E: Create,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/rjeczalik/fs",
					E: Create,
				},
			},
		},
		// i=1
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs/cmd/gotree/...",
				C: ch[0],
				E: Remove,
			},
			Record: []Call{
				{
					F:  FuncRecursiveRewatch,
					P:  "src/github.com/rjeczalik/fs",
					NP: "src/github.com/rjeczalik/fs",
					E:  Create,
					NE: Create | Remove,
				},
			},
		},
	}

	n.ExpectRecordedCalls(watches[:])

	events := [...]TCase{
		// i=0
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/.fs.go.swp", E: Create},
			Receiver: Chans{ch[0]},
		},
		// i=1
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/.fs.go.swp", E: Remove},
			Receiver: nil,
		},
		// i=2
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs", E: Remove},
			Receiver: nil,
		},
		// i=3
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/gotree/main.go", E: Remove},
			Receiver: Chans{ch[0]},
		},
	}

	n.ExpectTreeEvents(events[:], ch)
}

func TestRecursiveTree_Windows(t *testing.T) {
	n := NewRecursiveTreeTest(t, "testdata/vfs.txt")
	defer n.Close()

	const ChangeFileName = Event(0x1)

	ch := NewChans(1)

	watches := [...]RCase{
		// i=0
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs",
				C: ch[0],
				E: ChangeFileName,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/rjeczalik/fs",
					E: ChangeFileName,
				},
			},
		},
	}

	n.ExpectRecordedCalls(watches[:])

	events := [...]TCase{
		// i=0
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs", E: ChangeFileName},
			Receiver: Chans{ch[0]},
		},
		// i=1
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/fs.go", E: ChangeFileName},
			Receiver: Chans{ch[0]},
		},
	}

	n.ExpectTreeEvents(events[:], ch)
}
