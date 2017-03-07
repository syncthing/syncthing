// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin,!kqueue

package notify_test

import (
	"log"

	"github.com/zillode/notify"
)

// This example shows how to use FSEvents-specifc event values.
func ExampleWatch_darwin() {
	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	c := make(chan notify.EventInfo, 1)

	// Set up a watchpoint listening for FSEvents-specific events within a
	// current working directory. Dispatch each FSEventsChangeOwner and FSEventsMount
	// events separately to c.
	if err := notify.Watch(".", c, notify.FSEventsChangeOwner, notify.FSEventsMount); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(c)

	// Block until an event is received.
	switch ei := <-c; ei.Event() {
	case notify.FSEventsChangeOwner:
		log.Println("The owner of", ei.Path(), "has changed.")
	case notify.FSEventsMount:
		log.Println("The path", ei.Path(), "has been mounted.")
	}
}

// This example shows how to work with EventInfo's underlying FSEvent struct.
// Investigating notify.(*FSEvent).Flags field we are able to say whether
// the event's path is a file or a directory and many more.
func ExampleWatch_darwinDirFileSymlink() {
	var must = func(err error) {
		if err != nil {
			log.Fatal(err)
		}
	}
	var stop = func(c ...chan<- notify.EventInfo) {
		for _, c := range c {
			notify.Stop(c)
		}
	}

	// Make the channels buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	dir := make(chan notify.EventInfo, 1)
	file := make(chan notify.EventInfo, 1)
	symlink := make(chan notify.EventInfo, 1)
	all := make(chan notify.EventInfo, 1)

	// Set up a single watchpoint listening for FSEvents-specific events on
	// multiple user-provided channels.
	must(notify.Watch(".", dir, notify.FSEventsIsDir))
	must(notify.Watch(".", file, notify.FSEventsIsFile))
	must(notify.Watch(".", symlink, notify.FSEventsIsSymlink))
	must(notify.Watch(".", all, notify.All))
	defer stop(dir, file, symlink, all)

	// Block until an event is received.
	select {
	case ei := <-dir:
		log.Println("The directory", ei.Path(), "has changed")
	case ei := <-file:
		log.Println("The file", ei.Path(), "has changed")
	case ei := <-symlink:
		log.Println("The symlink", ei.Path(), "has changed")
	case ei := <-all:
		var kind string

		// Investigate underlying *notify.FSEvent struct to access more
		// information about the event.
		switch flags := ei.Sys().(*notify.FSEvent).Flags; {
		case flags&notify.FSEventsIsFile != 0:
			kind = "file"
		case flags&notify.FSEventsIsDir != 0:
			kind = "dir"
		case flags&notify.FSEventsIsSymlink != 0:
			kind = "symlink"
		}

		log.Printf("The %s under path %s has been %sd\n", kind, ei.Path(), ei.Event())
	}
}

// FSEvents may report multiple filesystem actions with one, coalesced event.
// Notify unscoalesces such event and dispatches series of single events
// back to the user.
//
// This example shows how to coalesce events by investigating notify.(*FSEvent).ID
// field, for the science.
func ExampleWatch_darwinCoalesce() {
	// Make the channels buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	c := make(chan notify.EventInfo, 4)

	// Set up a watchpoint listetning for events within current working directory.
	// Dispatch all platform-independent separately to c.
	if err := notify.Watch(".", c, notify.All); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(c)

	var id uint64
	var coalesced []notify.EventInfo

	for ei := range c {
		switch n := ei.Sys().(*notify.FSEvent).ID; {
		case id == 0:
			id = n
			coalesced = []notify.EventInfo{ei}
		case id == n:
			coalesced = append(coalesced, ei)
		default:
			log.Printf("FSEvents reported a filesystem action with the following"+
				" coalesced events %v groupped by %d ID\n", coalesced, id)
			return
		}
	}
}
