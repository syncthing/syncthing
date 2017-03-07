// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build windows

package notify

import "testing"

func TestNotifySystemSpecificEvent(t *testing.T) {
	n := NewNotifyTest(t, "testdata/vfs.txt")
	defer n.Close()

	ch := NewChans(1)

	n.Watch("src/github.com/rjeczalik/fs", ch[0], FileNotifyChangeFileName, FileNotifyChangeSize)

	cases := []NCase{
		{
			Event:    rremove(n.W(), "src/github.com/rjeczalik/fs/fs.go"),
			Receiver: Chans{ch[0]},
		},
		{
			Event:    rwrite(n.W(), "src/github.com/rjeczalik/fs/README.md", []byte("XD")),
			Receiver: Chans{ch[0]},
		},
	}

	n.ExpectNotifyEvents(cases, ch)
}

func TestUnknownEvent(t *testing.T) {
	n := NewNotifyTest(t, "testdata/vfs.txt")
	defer n.Close()

	ch := NewChans(1)

	n.WatchErr("src/github.com/rjeczalik/fs", ch[0], nil, FileActionAdded)
}

func TestNotifySystemAndGlobalMix(t *testing.T) {
	n := NewNotifyTest(t, "testdata/vfs.txt")
	defer n.Close()

	ch := NewChans(3)

	n.Watch("src/github.com/rjeczalik/fs", ch[0], Create)
	n.Watch("src/github.com/rjeczalik/fs", ch[1], FileNotifyChangeFileName)
	n.Watch("src/github.com/rjeczalik/fs", ch[2], FileNotifyChangeDirName)

	cases := []NCase{
		{
			Event:    rcreate(n.W(), "src/github.com/rjeczalik/fs/.main.cc.swr"),
			Receiver: Chans{ch[0], ch[1]},
		},
	}

	n.ExpectNotifyEvents(cases, ch)
}
