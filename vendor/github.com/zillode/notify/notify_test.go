// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin linux freebsd dragonfly netbsd openbsd windows solaris

package notify

import "testing"

func TestNotifyExample(t *testing.T) {
	n := NewNotifyTest(t, "testdata/vfs.txt")
	defer n.Close()

	ch := NewChans(3)

	// Watch-points can be set explicitly via Watch/Stop calls...
	n.Watch("src/github.com/rjeczalik/fs", ch[0], Write)
	n.Watch("src/github.com/pblaszczyk/qttu", ch[0], Write)
	n.Watch("src/github.com/pblaszczyk/qttu/...", ch[1], Create)
	n.Watch("src/github.com/rjeczalik/fs/cmd/...", ch[2], Remove)

	cases := []NCase{
		// i=0
		{
			Event:    write(n.W(), "src/github.com/rjeczalik/fs/fs.go", []byte("XD")),
			Receiver: Chans{ch[0]},
		},
		// TODO(rjeczalik): #62
		// i=1
		// {
		//	Event:    write(n.W(), "src/github.com/pblaszczyk/qttu/README.md", []byte("XD")),
		//	Receiver: Chans{ch[0]},
		// },
		// i=2
		{
			Event:    write(n.W(), "src/github.com/rjeczalik/fs/cmd/gotree/go.go", []byte("XD")),
			Receiver: nil,
		},
		// i=3
		{
			Event:    create(n.W(), "src/github.com/pblaszczyk/qttu/src/.main.cc.swp"),
			Receiver: Chans{ch[1]},
		},
		// i=4
		{
			Event:    create(n.W(), "src/github.com/pblaszczyk/qttu/src/.main.cc.swo"),
			Receiver: Chans{ch[1]},
		},
		// i=5
		{
			Event:    remove(n.W(), "src/github.com/rjeczalik/fs/cmd/gotree/go.go"),
			Receiver: Chans{ch[2]},
		},
	}

	n.ExpectNotifyEvents(cases, ch)

	// ...or using Call structures.
	stops := [...]Call{
		// i=0
		{
			F: FuncStop,
			C: ch[0],
		},
		// i=1
		{
			F: FuncStop,
			C: ch[1],
		},
	}

	n.Call(stops[:]...)

	cases = []NCase{
		// i=0
		{
			Event:    write(n.W(), "src/github.com/rjeczalik/fs/fs.go", []byte("XD")),
			Receiver: nil,
		},
		// i=1
		{
			Event:    write(n.W(), "src/github.com/pblaszczyk/qttu/README.md", []byte("XD")),
			Receiver: nil,
		},
		// i=2
		{
			Event:    create(n.W(), "src/github.com/pblaszczyk/qttu/src/.main.cc.swr"),
			Receiver: nil,
		},
		// i=3
		{
			Event:    remove(n.W(), "src/github.com/rjeczalik/fs/cmd/gotree/main.go"),
			Receiver: Chans{ch[2]},
		},
	}

	n.ExpectNotifyEvents(cases, ch)
}

func TestStop(t *testing.T) {
	t.Skip("TODO(rjeczalik)")
}
