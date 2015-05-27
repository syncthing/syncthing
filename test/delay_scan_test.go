// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"io/ioutil"
	"log"
	"sync"
	"testing"
	"time"
)

func TestDelayScan(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "h1/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 50, 18, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating .stignore...")
	err = ioutil.WriteFile("s1/.stignore", []byte("some ignore data\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Starting up...")
	st := syncthingProcess{ // id1
		instance: "1",
		argv:     []string{"-home", "h1"},
		port:     8081,
		apiKey:   apiKey,
	}
	err = st.start()
	if err != nil {
		t.Fatal(err)
	}

	// Wait for one scan to succeed, or up to 20 seconds...
	// This is to let startup, UPnP etc complete.
	for i := 0; i < 20; i++ {
		err := st.rescan("default")
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		break
	}

	// Wait for UPnP and stuff
	time.Sleep(10 * time.Second)

	var wg sync.WaitGroup
	log.Println("Starting scans...")
	for j := 0; j < 20; j++ {
		j := j
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := st.rescanNext("default", time.Duration(1)*time.Second)
			log.Println(j)
			if err != nil {
				log.Println(err)
				t.Fatal(err)
			}
		}()
	}

	wg.Wait()
	log.Println("Scans done")
	time.Sleep(2 * time.Second)

	// This is where the real test is currently, since stop() checks for data
	// race output in the log.
	log.Println("Stopping...")
	_, err = st.stop()
	if err != nil {
		t.Fatal(err)
	}
}
