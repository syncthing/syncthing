// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

import (
	"fmt"
	"testing"
)

func TestNonrecursiveTree(t *testing.T) {
	n := NewNonrecursiveTreeTest(t, "testdata/vfs.txt")
	defer n.Close()

	ch := NewChans(5)

	watches := [...]RCase{
		// i=0
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs/fs.go",
				C: ch[0],
				E: Rename,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/rjeczalik/fs/fs.go",
					E: Rename,
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
					F: FuncWatch,
					P: "src/github.com/rjeczalik/fs/cmd",
					E: Create | Remove,
				},
				{
					F: FuncWatch,
					P: "src/github.com/rjeczalik/fs/cmd/gotree",
					E: Create | Remove,
				},
				{
					F: FuncWatch,
					P: "src/github.com/rjeczalik/fs/cmd/mktree",
					E: Create | Remove,
				},
			},
		},
		// i=2
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs/cmd/...",
				C: ch[2],
				E: Rename,
			},
			Record: []Call{
				{
					F:  FuncRewatch,
					P:  "src/github.com/rjeczalik/fs/cmd",
					E:  Create | Remove,
					NE: Create | Remove | Rename,
				},
				{
					F:  FuncRewatch,
					P:  "src/github.com/rjeczalik/fs/cmd/gotree",
					E:  Create | Remove,
					NE: Create | Remove | Rename,
				},
				{
					F:  FuncRewatch,
					P:  "src/github.com/rjeczalik/fs/cmd/mktree",
					E:  Create | Remove,
					NE: Create | Remove | Rename,
				},
			},
		},
		// i=3
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs/cmd/mktree/...",
				C: ch[2],
				E: Write,
			},
			Record: []Call{
				{
					F:  FuncRewatch,
					P:  "src/github.com/rjeczalik/fs/cmd/mktree",
					E:  Create | Remove | Rename,
					NE: Create | Remove | Rename | Write,
				},
			},
		},
		// i=4
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/pblaszczyk/qttu/include",
				C: ch[3],
				E: Create,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/pblaszczyk/qttu/include",
					E: Create,
				},
			},
		},
		// i=5
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/pblaszczyk/qttu/include/qttu/detail/...",
				C: ch[3],
				E: Write,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/pblaszczyk/qttu/include/qttu/detail",
					E: Create | Write,
				},
			},
		},
		// i=6
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/pblaszczyk/qttu/include/...",
				C: ch[0],
				E: Rename,
			},
			Record: []Call{
				{
					F:  FuncRewatch,
					P:  "src/github.com/pblaszczyk/qttu/include",
					E:  Create,
					NE: Create | Rename,
				},
				{
					F: FuncWatch,
					P: "src/github.com/pblaszczyk/qttu/include/qttu",
					E: Create | Rename,
				},
				{
					F:  FuncRewatch,
					P:  "src/github.com/pblaszczyk/qttu/include/qttu/detail",
					E:  Create | Write,
					NE: Create | Write | Rename,
				},
			},
		},
		// i=7
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/pblaszczyk/...",
				C: ch[1],
				E: Write,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/pblaszczyk",
					E: Create | Write,
				},
				{
					F: FuncWatch,
					P: "src/github.com/pblaszczyk/qttu",
					E: Create | Write,
				},
				{
					F:  FuncRewatch,
					P:  "src/github.com/pblaszczyk/qttu/include",
					E:  Create | Rename,
					NE: Create | Rename | Write,
				},
				{
					F:  FuncRewatch,
					P:  "src/github.com/pblaszczyk/qttu/include/qttu",
					E:  Create | Rename,
					NE: Create | Rename | Write,
				},
				{
					F: FuncWatch,
					P: "src/github.com/pblaszczyk/qttu/src",
					E: Create | Write,
				},
			},
		},
		// i=8
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/pblaszczyk/qttu/include/...",
				C: ch[4],
				E: Write,
			},
			Record: nil,
		},
		// i=9
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/pblaszczyk/qttu",
				C: ch[3],
				E: Remove,
			},
			Record: []Call{
				{
					F:  FuncRewatch,
					P:  "src/github.com/pblaszczyk/qttu",
					E:  Create | Write,
					NE: Create | Write | Remove,
				},
			},
		},
	}

	n.ExpectRecordedCalls(watches[:])

	events := [...]TCase{
		// i=0
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/fs.go", E: Rename},
			Receiver: Chans{ch[0]},
		},
		// i=1
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/fs.go", E: Create},
			Receiver: nil,
		},
		// i=2
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/cmd.go", E: Remove},
			Receiver: Chans{ch[1]},
		},
		// i=3
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/doc.go", E: Write},
			Receiver: nil,
		},
		// i=4
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/mktree/main.go", E: Write},
			Receiver: Chans{ch[2]},
		},
		// i=5
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/mktree/tree.go", E: Create},
			Receiver: nil,
		},
		// i=6
		{
			Event:    Call{P: "src/github.com/pblaszczyk/qttu/include/.lock", E: Create},
			Receiver: Chans{ch[3]},
		},
		// i=7
		{
			Event:    Call{P: "src/github.com/pblaszczyk/qttu/include/qttu/detail/registry.hh", E: Write},
			Receiver: Chans{ch[3], ch[1], ch[4]},
		},
		// i=8
		{
			Event:    Call{P: "src/github.com/pblaszczyk/qttu/include/qttu", E: Remove},
			Receiver: nil,
		},
		// i=9
		{
			Event:    Call{P: "src/github.com/pblaszczyk/qttu/include", E: Remove},
			Receiver: Chans{ch[3]},
		},
	}

	n.ExpectTreeEvents(events[:], ch)

	stops := [...]RCase{
		// i=0
		{
			Call: Call{
				F: FuncStop,
				C: ch[4],
			},
			Record: nil,
		},
		// i=1
		{
			Call: Call{
				F: FuncStop,
				C: ch[3],
			},
			Record: []Call{
				{
					F:  FuncRewatch,
					P:  "src/github.com/pblaszczyk/qttu",
					E:  Create | Write | Remove,
					NE: Create | Write,
				},
			},
		},
		// i=2
		{
			Call: Call{
				F: FuncStop,
				C: ch[2],
			},
			Record: []Call{
				{
					F:  FuncRewatch,
					P:  "src/github.com/rjeczalik/fs/cmd",
					E:  Create | Remove | Rename,
					NE: Create | Remove,
				},
				{
					F:  FuncRewatch,
					P:  "src/github.com/rjeczalik/fs/cmd/gotree",
					E:  Create | Remove | Rename,
					NE: Create | Remove,
				},
				{
					F:  FuncRewatch,
					P:  "src/github.com/rjeczalik/fs/cmd/mktree",
					E:  Create | Remove | Rename | Write,
					NE: Create | Remove,
				},
			},
		},
		// i=3
		{
			Call: Call{
				F: FuncStop,
				C: ch[1],
			},
			Record: []Call{
				{
					F: FuncUnwatch,
					P: "src/github.com/pblaszczyk",
				},
				{
					F: FuncUnwatch,
					P: "src/github.com/pblaszczyk/qttu",
				},
				{
					F:  FuncRewatch,
					P:  "src/github.com/pblaszczyk/qttu/include",
					E:  Create | Rename | Write,
					NE: Create | Rename,
				},
				{
					F:  FuncRewatch,
					P:  "src/github.com/pblaszczyk/qttu/include/qttu",
					E:  Create | Rename | Write,
					NE: Create | Rename,
				},
				{
					F:  FuncRewatch,
					P:  "src/github.com/pblaszczyk/qttu/include/qttu/detail",
					E:  Create | Rename | Write,
					NE: Create | Rename,
				},
				{
					F: FuncUnwatch,
					P: "src/github.com/pblaszczyk/qttu/src",
				},
				{
					F: FuncUnwatch,
					P: "src/github.com/rjeczalik/fs/cmd",
				},
				{
					F: FuncUnwatch,
					P: "src/github.com/rjeczalik/fs/cmd/gotree",
				},
				{
					F: FuncUnwatch,
					P: "src/github.com/rjeczalik/fs/cmd/mktree",
				},
			},
		},
		// i=4
		{
			Call: Call{
				F: FuncStop,
				C: ch[0],
			},
			Record: []Call{
				{
					F: FuncUnwatch,
					P: "src/github.com/pblaszczyk/qttu/include",
				},
				{
					F: FuncUnwatch,
					P: "src/github.com/pblaszczyk/qttu/include/qttu",
				},
				{
					F: FuncUnwatch,
					P: "src/github.com/pblaszczyk/qttu/include/qttu/detail",
				},
				{
					F: FuncUnwatch,
					P: "src/github.com/rjeczalik/fs/fs.go",
				},
			},
		},
	}

	n.ExpectRecordedCalls(stops[:])

	n.Walk(func(nd node) error {
		if len(nd.Watch) != 0 {
			return fmt.Errorf("unexpected watchpoint: name=%s, eventset=%v (len=%d)",
				nd.Name, nd.Watch.Total(), len(nd.Watch))
		}
		return nil
	})
}

func TestNonrecursiveTreeInternal(t *testing.T) {
	n, c := NewNonrecursiveTreeTestC(t, "testdata/vfs.txt")
	defer n.Close()

	ch := NewChans(5)

	watches := [...]RCase{
		// i=0
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/rjeczalik/fs/cmd/...",
				C: ch[0],
				E: Remove,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/rjeczalik/fs/cmd",
					E: Create | Remove,
				},
				{
					F: FuncWatch,
					P: "src/github.com/rjeczalik/fs/cmd/gotree",
					E: Create | Remove,
				},
				{
					F: FuncWatch,
					P: "src/github.com/rjeczalik/fs/cmd/mktree",
					E: Create | Remove,
				},
			},
		},
		// i=1
		{
			Call: Call{
				F: FuncWatch,
				P: "src/github.com/ppknap/link/include/coost/...",
				C: ch[1],
				E: Create,
			},
			Record: []Call{
				{
					F: FuncWatch,
					P: "src/github.com/ppknap/link/include/coost",
					E: Create,
				},
				{
					F: FuncWatch,
					P: "src/github.com/ppknap/link/include/coost/link",
					E: Create,
				},
				{
					F: FuncWatch,
					P: "src/github.com/ppknap/link/include/coost/link/detail",
					E: Create,
				},
				{
					F: FuncWatch,
					P: "src/github.com/ppknap/link/include/coost/link/detail/stdhelpers",
					E: Create,
				},
			},
		},
	}

	n.ExpectRecordedCalls(watches[:])

	events := [...]TCase{
		// i=0
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/dir", E: Create, Dir: true},
			Receiver: Chans{c},
		},
		// i=1
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/dir/another", E: Create, Dir: true},
			Receiver: Chans{c},
		},
		// i=2
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/file", E: Create, Dir: false},
			Receiver: nil,
		},
		// i=3
		{
			Event:    Call{P: "src/github.com/ppknap/link/include/coost/dir", E: Create, Dir: true},
			Receiver: Chans{ch[1], c},
		},
		// i=4
		{
			Event:    Call{P: "src/github.com/ppknap/link/include/coost/dir/another", E: Create, Dir: true},
			Receiver: Chans{ch[1], c},
		},
		// i=5
		{
			Event:    Call{P: "src/github.com/ppknap/link/include/coost/file", E: Create, Dir: false},
			Receiver: Chans{ch[1]},
		},
		// i=6
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/mktree", E: Remove},
			Receiver: Chans{ch[0]},
		},
		// i=7
		{
			Event:    Call{P: "src/github.com/rjeczalik/fs/cmd/rmtree", E: Create, Dir: true},
			Receiver: Chans{c},
		},
	}

	n.ExpectTreeEvents(events[:], ch)
}
