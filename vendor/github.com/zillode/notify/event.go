// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify

import (
	"fmt"
	"strings"
)

// Event represents the type of filesystem action.
//
// Number of available event values is dependent on the target system or the
// watcher implmenetation used (e.g. it's possible to use either kqueue or
// FSEvents on Darwin).
//
// Please consult documentation for your target platform to see list of all
// available events.
type Event uint32

// Create, Remove, Write and Rename are the only event values guaranteed to be
// present on all platforms.
const (
	Create = osSpecificCreate
	Remove = osSpecificRemove
	Write  = osSpecificWrite
	Rename = osSpecificRename

	// All is handful alias for all platform-independent event values.
	All = Create | Remove | Write | Rename
)

const internal = recursive | omit

// String implements fmt.Stringer interface.
func (e Event) String() string {
	var s []string
	for _, strmap := range []map[Event]string{estr, osestr} {
		for ev, str := range strmap {
			if e&ev == ev {
				s = append(s, str)
			}
		}
	}
	return strings.Join(s, "|")
}

// EventInfo describes an event reported by the underlying filesystem notification
// subsystem.
//
// It always describes single event, even if the OS reported a coalesced action.
// Reported path is absolute and clean.
//
// For non-recursive watchpoints its base is always equal to the path passed
// to corresponding Watch call.
//
// The value of Sys if system-dependent and can be nil.
//
// Sys
//
// Under Darwin (FSEvents) Sys() always returns a non-nil *notify.FSEvent value,
// which is defined as:
//
//   type FSEvent struct {
//       Path  string // real path of the file or directory
//       ID    uint64 // ID of the event (FSEventStreamEventId)
//       Flags uint32 // joint FSEvents* flags (FSEventStreamEventFlags)
//   }
//
// For possible values of Flags see Darwin godoc for notify or FSEvents
// documentation for FSEventStreamEventFlags constants:
//
//    https://developer.apple.com/library/mac/documentation/Darwin/Reference/FSEvents_Ref/index.html#//apple_ref/doc/constant_group/FSEventStreamEventFlags
//
// Under Linux (inotify) Sys() always returns a non-nil *syscall.InotifyEvent
// value, defined as:
//
//   type InotifyEvent struct {
//       Wd     int32    // Watch descriptor
//       Mask   uint32   // Mask describing event
//       Cookie uint32   // Unique cookie associating related events (for rename(2))
//       Len    uint32   // Size of name field
//       Name   [0]uint8 // Optional null-terminated name
//   }
//
// More information about inotify masks and the usage of inotify_event structure
// can be found at:
//
//    http://man7.org/linux/man-pages/man7/inotify.7.html
//
// Under Darwin, DragonFlyBSD, FreeBSD, NetBSD, OpenBSD (kqueue) Sys() always
// returns a non-nil *notify.Kevent value, which is defined as:
//
//   type Kevent struct {
//       Kevent *syscall.Kevent_t // Kevent is a kqueue specific structure
//       FI     os.FileInfo       // FI describes file/dir
//   }
//
// More information about syscall.Kevent_t can be found at:
//
//    https://www.freebsd.org/cgi/man.cgi?query=kqueue
//
// Under Windows (ReadDirectoryChangesW) Sys() always returns nil. The documentation
// of watcher's WinAPI function can be found at:
//
//    https://msdn.microsoft.com/en-us/library/windows/desktop/aa365465%28v=vs.85%29.aspx
type EventInfo interface {
	Event() Event     // event value for the filesystem action
	Path() string     // real path of the file or directory
	Sys() interface{} // underlying data source (can return nil)
}

type isDirer interface {
	isDir() (bool, error)
}

var _ fmt.Stringer = (*event)(nil)
var _ isDirer = (*event)(nil)

// String implements fmt.Stringer interface.
func (e *event) String() string {
	return e.Event().String() + `: "` + e.Path() + `"`
}

var estr = map[Event]string{
	Create: "notify.Create",
	Remove: "notify.Remove",
	Write:  "notify.Write",
	Rename: "notify.Rename",
	// Display name for recursive event is added only for debugging
	// purposes. It's an internal event after all and won't be exposed to the
	// user. Having Recursive event printable is helpful, e.g. for reading
	// testing failure messages:
	//
	//    --- FAIL: TestWatchpoint (0.00 seconds)
	//    watchpoint_test.go:64: want diff=[notify.Remove notify.Create|notify.Remove];
	//    got [notify.Remove notify.Remove|notify.Create] (i=1)
	//
	// Yup, here the diff have Recursive event inside. Go figure.
	recursive: "recursive",
	omit:      "omit",
}
