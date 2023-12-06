// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build integration
// +build integration

package integration

import (
	"log"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/rc"
)

func TestReconnectReceiverDuringTransfer(t *testing.T) {
	testReconnectDuringTransfer(t, false, true)
}

func TestReconnectSenderDuringTransfer(t *testing.T) {
	testReconnectDuringTransfer(t, true, false)
}

func testReconnectDuringTransfer(t *testing.T, restartSender, restartReceiver bool) {
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
		// We need a closure over sender, since we'll update it later to
		// point at another process.
		checkedStop(t, receiver)
	}()

	// Set rate limits
	cfg, err := receiver.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Options.MaxRecvKbps = 750
	cfg.Options.MaxSendKbps = 750
	cfg.Options.LimitBandwidthInLan = true
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

		if recv.InSyncBytes > 0 && recv.InSyncBytes == recv.GlobalBytes && rc.InSync("default", receiver, sender) {
			// Receiver is done
			break
		} else if recv.InSyncBytes > prevBytes+recv.GlobalBytes/10 {
			// Receiver has made progress
			prevBytes = recv.InSyncBytes

			if restartReceiver {
				log.Printf("Stopping receiver...")
				receiver.Stop()
				receiver = startInstance(t, 2)
				receiver.ResumeAll()
			}

			if restartSender {
				log.Printf("Stopping sender...")
				sender.Stop()
				sender = startInstance(t, 1)
				sender.ResumeAll()
			}
		}

		time.Sleep(250 * time.Millisecond)
	}

	// Reset rate limits
	cfg, err = receiver.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Options.MaxRecvKbps = 0
	cfg.Options.MaxSendKbps = 0
	cfg.Options.LimitBandwidthInLan = false
	if err := receiver.PostConfig(cfg); err != nil {
		t.Fatal(err)
	}

	checkedStop(t, sender)
	checkedStop(t, receiver)

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Error(err)
	}

	if err := checkRemoteInSync("default", receiver, sender); err != nil {
		t.Error(err)
	}
}
