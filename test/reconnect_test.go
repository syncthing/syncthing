// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// +build integration

package integration_test

import (
	"log"
	"strings"
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
	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}

	receiver := syncthingProcess{ // id2
		log:    "2.out",
		argv:   []string{"-home", "h2"},
		port:   8082,
		apiKey: apiKey,
	}
	err = receiver.start()
	if err != nil {
		sender.stop()
		t.Fatal(err)
	}

	var prevComp int
	for {
		comp, err := sender.peerCompletion()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				time.Sleep(250 * time.Millisecond)
				continue
			}
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

		time.Sleep(250 * time.Millisecond)
	}

	sender.stop()
	receiver.stop()

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}
