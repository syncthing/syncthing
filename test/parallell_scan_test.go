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

func TestParallellScan(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "h1/index")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 5000, 18, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generaing .stignore...")
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
	time.Sleep(5 * time.Second)

	var wg sync.WaitGroup
	log.Println("Starting scans...")
	for j := 0; j < 20; j++ {
		j := j
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := st.post("/rest/scan?folder=default", nil)
			log.Println(j)
			if err != nil {
				log.Println(err)
				t.Fatal(err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("%d != 200", resp.StatusCode)
			}
			resp.Body.Close()
		}()
	}

	wg.Wait()
	log.Println("Scans done")
	time.Sleep(2 * time.Second)

	// This is where the real test is currently, since stop() checks for data
	// race output in the log.
	log.Println("Stopping...")
	err = st.stop()
	if err != nil {
		t.Fatal(err)
	}
}
