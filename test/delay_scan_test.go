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
	"os"
	"sync"
	"testing"
	"time"
)

func TestRescanWithDelay(t *testing.T) {
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
	err = os.WriteFile("s1/.stignore", []byte("some ignore data\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Starting up...")

	st := startInstance(t, 1)

	var wg sync.WaitGroup
	log.Println("Starting scans...")
	for j := 0; j < 20; j++ {
		j := j
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := st.RescanDelay("default", 1)
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
	checkedStop(t, st)
}
