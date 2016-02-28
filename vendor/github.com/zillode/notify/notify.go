// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// BUG(rjeczalik): Notify does not collect watchpoints, when underlying watches
// were removed by their os-specific watcher implementations. Instead users are
// advised to listen on persistant paths to have guarantee they receive events
// for the whole lifetime of their applications (to discuss see #69).

// BUG(ppknap): Linux (inotify) does not support watcher behavior masks like
// InOneshot, InOnlydir etc. Instead users are advised to perform the filtering
// themselves (to discuss see #71).

// BUG(ppknap): Notify  was not tested for short path name support under Windows
// (ReadDirectoryChangesW).

// BUG(ppknap): Windows (ReadDirectoryChangesW) cannot recognize which notification
// triggers FileActionModified event. (to discuss see #75).

package notify

var defaultTree = newTree()

// Watch sets up a watchpoint on path listening for events given by the events
// argument.
//
// File or directory given by the path must exist, otherwise Watch will fail
// with non-nil error. Notify resolves, for its internal purpose, any symlinks
// the provided path may contain, so it may fail if the symlinks form a cycle.
// It does so, since not all watcher implementations treat passed paths as-is.
// E.g. FSEvents reports a real path for every event, setting a watchpoint
// on /tmp will report events with paths rooted at /private/tmp etc.
//
// The c almost always is a buffered channel. Watch will not block sending to c
// - the caller must ensure that c has sufficient buffer space to keep up with
// the expected event rate.
//
// It is allowed to pass the same channel multiple times with different event
// list or different paths. Calling Watch with different event lists for a single
// watchpoint expands its event set. The only way to shrink it, is to call
// Stop on its channel.
//
// Calling Watch with empty event list does expand nor shrink watchpoint's event
// set. If c is the first channel to listen for events on the given path, Watch
// will seamlessly create a watch on the filesystem.
//
// Notify dispatches copies of single filesystem event to all channels registered
// for each path. If a single filesystem event contains multiple coalesced events,
// each of them is dispatched separately. E.g. the following filesystem change:
//
//   ~ $ echo Hello > Notify.txt
//
// dispatches two events - notify.Create and notify.Write. However, it may depend
// on the underlying watcher implementation whether OS reports both of them.
//
// Windows and recursive watches
//
// If a directory which path was used to create recursive watch under Windows
// gets deleted, the OS will not report such event. It is advised to keep in
// mind this limitation while setting recursive watchpoints for your application,
// e.g. use persistant paths like %userprofile% or watch additionally parent
// directory of a recursive watchpoint in order to receive delete events for it.
func Watch(path string, c chan<- EventInfo, events ...Event) error {
	return defaultTree.Watch(path, c, events...)
}

// Stop removes all watchpoints registered for c. All underlying watches are
// also removed, for which c was the last channel listening for events.
//
// Stop does not close c. When Stop returns, it is guranteed that c will
// receive no more signals.
func Stop(c chan<- EventInfo) {
	defaultTree.Stop(c)
}
