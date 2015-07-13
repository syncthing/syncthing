// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"log"
	"sync"
	"testing"
	"time"
)

func TestRestartReceiverDuringTransfer(t *testing.T) {
	testRestartDuringTransfer(t, false, true, 0, 0)
}

func TestRestartSenderDuringTransfer(t *testing.T) {
	testRestartDuringTransfer(t, true, false, 0, 0)
}

func TestRestartSenderAndReceiverDuringTransfer(t *testing.T) {
	// Give the receiver some time to rot with needed files but
	// without any peer. This triggers
	// https://github.com/syncthing/syncthing/issues/463
	testRestartDuringTransfer(t, true, true, 10*time.Second, 0)
}

func testRestartDuringTransfer(t *testing.T, restartSender, restartReceiver bool, senderDelay, receiverDelay time.Duration) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 250, 20, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Starting up...")
	sender := startInstance(t, 1)
	defer func() {
		// We need a closure over sender, since we'll update it later to point
		// at another process.
		checkedStop(t, sender)
	}()

	receiver := startInstance(t, 2)
	defer func() {
		// We need a receiver over sender, since we'll update it later to
		// point at another process.
		checkedStop(t, receiver)
	}()

	var prevBytes int
	for {
		recv, err := receiver.Model("default")
		if err != nil {
			t.Fatal(err)
		}

		if recv.InSyncBytes > 0 && recv.InSyncBytes == recv.GlobalBytes {
			// Receiver is done
			break
		} else if recv.InSyncBytes > prevBytes+recv.GlobalBytes/10 {
			// Receiver has made progress
			prevBytes = recv.InSyncBytes

			if restartReceiver {
				log.Printf("Stopping receiver...")
				checkedStop(t, receiver)
			}

			if restartSender {
				log.Printf("Stopping sender...")
				checkedStop(t, sender)
			}

			var wg sync.WaitGroup

			if restartReceiver {
				wg.Add(1)
				go func() {
					time.Sleep(receiverDelay)
					receiver = startInstance(t, 2)
					wg.Done()
				}()
			}

			if restartSender {
				wg.Add(1)
				go func() {
					time.Sleep(senderDelay)
					sender = startInstance(t, 1)
					wg.Done()
				}()
			}

			wg.Wait()
		}

		time.Sleep(time.Second)
	}

	checkedStop(t, sender)
	checkedStop(t, receiver)

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}
