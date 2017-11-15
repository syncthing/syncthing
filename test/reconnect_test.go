// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"log"
	"sync"
	"testing"
	"time"
)

func TestReconnectReceiverDuringTransfer(t *testing.T) {
	testReconnectDuringTransfer(t, false, true, 0, 0)
}

func TestReconnectSenderDuringTransfer(t *testing.T) {
	testReconnectDuringTransfer(t, true, false, 0, 0)
}

func TestReconnectSenderAndReceiverDuringTransfer(t *testing.T) {
	// Give the receiver some time to rot with needed files but
	// without any peer. This triggers
	// https://github.com/syncthing/syncthing/issues/463
	testReconnectDuringTransfer(t, true, true, 10*time.Second, 0)
}

func testReconnectDuringTransfer(t *testing.T, ReconnectSender, ReconnectReceiver bool, senderDelay, receiverDelay time.Duration) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 2500, 20, "../LICENSE")
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

	// Set rate limits
	cfg, err := receiver.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Options.MaxRecvKbps = 100
	cfg.Options.MaxSendKbps = 100
	if err := receiver.PostConfig(cfg); err != nil {
		t.Fatal(err)
	}

	sender.ResumeAll()
	receiver.ResumeAll()

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

			if ReconnectReceiver {
				log.Printf("Pausing receiver...")
				receiver.PauseAll()
			}

			if ReconnectSender {
				log.Printf("Pausing sender...")
				sender.PauseAll()
			}

			var wg sync.WaitGroup

			if ReconnectReceiver {
				wg.Add(1)
				go func() {
					time.Sleep(receiverDelay)
					log.Printf("Resuming receiver...")
					receiver.ResumeAll()
					wg.Done()
				}()
			}

			if ReconnectSender {
				wg.Add(1)
				go func() {
					time.Sleep(senderDelay)
					log.Printf("Resuming sender...")
					sender.ResumeAll()
					wg.Done()
				}()
			}

			wg.Wait()
		}

		time.Sleep(time.Second)
	}

	// Reset rate limits
	cfg, err = receiver.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Options.MaxRecvKbps = 0
	cfg.Options.MaxSendKbps = 0
	if err := receiver.PostConfig(cfg); err != nil {
		t.Fatal(err)
	}

	checkedStop(t, sender)
	checkedStop(t, receiver)

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}
