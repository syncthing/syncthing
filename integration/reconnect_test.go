// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build integration

package integration_test

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
	err := removeAll("s1", "s2", "h1/index", "h2/index")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 1000, 22, "../bin/syncthing")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Starting up...")
	sender := syncthingProcess{ // id1
		log:    "1.out",
		argv:   []string{"-home", "h1"},
		port:   8081,
		apiKey: apiKey,
	}
	ver, err := sender.start()
	if err != nil {
		t.Fatal(err)
	}
	log.Println(ver)

	receiver := syncthingProcess{ // id2
		log:    "2.out",
		argv:   []string{"-home", "h2"},
		port:   8082,
		apiKey: apiKey,
	}
	ver, err = receiver.start()
	if err != nil {
		sender.stop()
		t.Fatal(err)
	}
	log.Println(ver)

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
				log.Printf("Stopping receiver...")
				receiver.stop()
			}

			if restartSender {
				log.Printf("Stopping sender...")
				sender.stop()
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

			prevComp = curComp
		}
	}

	sender.stop()
	receiver.stop()

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}
