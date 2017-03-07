// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

package notify_test

import (
	"log"
	"path/filepath"
	"time"

	"github.com/zillode/notify"
)

// This is a basic example showing how to work with notify.Watch function.
func ExampleWatch() {
	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	c := make(chan notify.EventInfo, 1)

	// Set up a watchpoint listening on events within current working directory.
	// Dispatch each create and remove events separately to c.
	if err := notify.Watch(".", c, notify.Create, notify.Remove); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(c)

	// Block until an event is received.
	ei := <-c
	log.Println("Got event:", ei)
}

// This example shows how to set up a recursive watchpoint.
func ExampleWatch_recursive() {
	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	c := make(chan notify.EventInfo, 1)

	// Set up a watchpoint listening for events within a directory tree rooted
	// at current working directory. Dispatch remove events to c.
	if err := notify.Watch("./...", c, notify.Remove); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(c)

	// Block until an event is received.
	ei := <-c
	log.Println("Got event:", ei)
}

// This example shows why it is important to not create leaks by stoping
// a channel when it's no longer being used.
func ExampleStop() {
	waitfor := func(path string, e notify.Event, timeout time.Duration) bool {
		dir, file := filepath.Split(path)
		c := make(chan notify.EventInfo, 1)

		if err := notify.Watch(dir, c, e); err != nil {
			log.Fatal(err)
		}
		// Clean up watchpoint associated with c. If Stop was not called upon
		// return the channel would be leaked as notify holds the only reference
		// to it and does not release it on its own.
		defer notify.Stop(c)

		t := time.After(timeout)

		for {
			select {
			case ei := <-c:
				if filepath.Base(ei.Path()) == file {
					return true
				}
			case <-t:
				return false
			}
		}
	}

	if waitfor("index.lock", notify.Create, 5*time.Second) {
		log.Println("The git repository was locked")
	}

	if waitfor("index.lock", notify.Remove, 5*time.Second) {
		log.Println("The git repository was unlocked")
	}
}
