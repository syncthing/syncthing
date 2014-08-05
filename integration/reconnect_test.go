// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build integration

package integration_test

import (
	"sync"
	"testing"
	"time"
)

const (
	apiKey = "abc123" // Used when talking to the processes under test
	id1    = "I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU"
	id2    = "JMFJCXB-GZDE4BN-OCJE3VF-65GYZNU-AIVJRET-3J6HMRQ-AUQIGJO-FKNHMQU"
)

var env = []string{
	"HOME=.",
	"STTRACE=model",
}

func TestRestartBothDuringTransfer(t *testing.T) {
	// Give the receiver some time to rot with needed files but
	// without any peer. This triggers
	// https://github.com/syncthing/syncthing/issues/463
	testRestartDuringTransfer(t, true, true, 10*time.Second, 0)
}

func TestRestartReceiverDuringTransfer(t *testing.T) {
	testRestartDuringTransfer(t, false, true, 0, 0)
}

func TestRestartSenderDuringTransfer(t *testing.T) {
	testRestartDuringTransfer(t, true, false, 0, 0)
}

func testRestartDuringTransfer(t *testing.T, restartSender, restartReceiver bool, senderDelay, receiverDelay time.Duration) {
	t.Log("Cleaning...")
	err := removeAll("s1", "s2", "f1/index", "f2/index")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Generating files...")
	err = generateFiles("s1", 1000, 20, "../bin/syncthing")
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Starting up...")
	sender := syncthingProcess{ // id1
		log:  "1.out",
		argv: []string{"-home", "f1"},
		port: 8081,
	}
	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}

	receiver := syncthingProcess{ // id2
		log:  "2.out",
		argv: []string{"-home", "f2"},
		port: 8082,
	}
	err = receiver.start()
	if err != nil {
		t.Fatal(err)
	}

	// Give them time to start up
	time.Sleep(1 * time.Second)

	var prevComp int
	for {
		comp, err := sender.peerCompletion()
		if err != nil {
			sender.stop()
			receiver.stop()
			t.Fatal(err)
		}

		curComp := comp[id2]

		if curComp == 100 {
			sender.stop()
			receiver.stop()
			break
		}

		if curComp > prevComp {
			if restartReceiver {
				t.Logf("Stopping receiver...")
				receiver.stop()
			}

			if restartSender {
				t.Logf("Stopping sender...")
				sender.stop()
			}

			var wg sync.WaitGroup

			if restartReceiver {
				wg.Add(1)
				go func() {
					time.Sleep(receiverDelay)
					t.Logf("Starting receiver...")
					receiver.start()
					wg.Done()
				}()
			}

			if restartSender {
				wg.Add(1)
				go func() {
					time.Sleep(senderDelay)
					t.Logf("Starting sender...")
					sender.start()
					wg.Done()
				}()
			}

			wg.Wait()

			prevComp = curComp
		}

		time.Sleep(1 * time.Second)
	}

	t.Log("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}
