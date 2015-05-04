// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

/*
This test just proves that write transactions may block.

func TestBoltTransactions(t *testing.T) {
	os.Remove("testdata/test.db")
	defer os.Remove("testdata/test.db")

	db, err := bolt.Open("testdata/test.db", 0644, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Set an initial value in a bucket

	err = db.Update(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucket([]byte("test"))
		if err != nil {
			return err
		}

		return bkt.Put([]byte("key"), []byte("avalue"))
	})
	if err != nil {
		t.Fatal(err)
	}

	// Start a long running read

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		db.View(func(tx *bolt.Tx) error {
			wg.Done()
			select {} // This read takes a long time... We're sending stuff over the network or something.
		})
	}()

	// Make sure the read has started
	wg.Wait()

	// Perform a nil update
	db.Update(func(tx *bolt.Tx) error {
		return nil
	})

	panic("this is never reached")
}
*/
