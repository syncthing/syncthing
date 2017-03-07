// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build windows

package notify_test

import (
	"log"

	"github.com/zillode/notify"
)

// This example shows how to watch directory-name changes in the working directory subtree.
func ExampleWatch_windows() {
	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	c := make(chan notify.EventInfo, 4)

	// Since notify package behaves exactly like ReadDirectoryChangesW function,
	// we must register notify.FileNotifyChangeDirName filter and wait for one
	// of FileAction* events.
	if err := notify.Watch("./...", c, notify.FileNotifyChangeDirName); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(c)

	// Wait for actions.
	for ei := range c {
		switch ei.Event() {
		case notify.FileActionAdded, notify.FileActionRenamedNewName:
			log.Println("Created:", ei.Path())
		case notify.FileActionRemoved, notify.FileActionRenamedOldName:
			log.Println("Removed:", ei.Path())
		case notify.FileActionModified:
			panic("notify: unexpected action")
		}
	}
}
