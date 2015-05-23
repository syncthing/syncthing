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
	sender := syncthingProcess{ // id1
		instance: "1",
		argv:     []string{"-home", "h1"},
		port:     8081,
		apiKey:   apiKey,
	}
	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}
	defer sender.stop()

	waitForScan(sender)

	receiver := syncthingProcess{ // id2
		instance: "2",
		argv:     []string{"-home", "h2"},
		port:     8082,
		apiKey:   apiKey,
	}
	err = receiver.start()
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.stop()

	var prevBytes int
	for {
		recv, err := receiver.dbStatus("default")
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
				_, err = receiver.stop()
				if err != nil {
					t.Fatal(err)
				}
			}

			if restartSender {
				log.Printf("Stopping sender...")
				_, err = sender.stop()
				if err != nil {
					t.Fatal(err)
				}
			}

			var wg sync.WaitGroup

			if restartReceiver {
				wg.Add(1)
				go func() {
					time.Sleep(receiverDelay)
					log.Printf("Starting receiver...")
					receiver.start()
					wg.Done()
				}()
			}

			if restartSender {
				wg.Add(1)
				go func() {
					time.Sleep(senderDelay)
					log.Printf("Starting sender...")
					sender.start()
					wg.Done()
				}()
			}

			wg.Wait()
		}

		time.Sleep(time.Second)
	}

	_, err = sender.stop()
	if err != nil {
		t.Fatal(err)
	}
	_, err = receiver.stop()
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}
