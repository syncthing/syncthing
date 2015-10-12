// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

func TestVirtualMtimeRepo(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// A few repos so we can ensure they don't pollute each other
	repo1 := NewVirtualMtimeRepo(ldb, "folder1")
	repo2 := NewVirtualMtimeRepo(ldb, "folder2")

	// Since GetMtime() returns its argument if the key isn't found or is outdated, we need a dummy to test with.
	dummyTime := time.Date(2001, time.February, 3, 4, 5, 6, 0, time.UTC)

	// Some times to test with
	time1 := time.Date(2001, time.February, 3, 4, 5, 7, 0, time.UTC)
	time2 := time.Date(2010, time.February, 3, 4, 5, 6, 0, time.UTC)

	file1 := "file1.txt"

	// Files are not present at the start

	if v := repo1.GetMtime(file1, dummyTime); !v.Equal(dummyTime) {
		t.Errorf("Mtime should be missing (%v) from repo 1 but it's %v", dummyTime, v)
	}

	if v := repo2.GetMtime(file1, dummyTime); !v.Equal(dummyTime) {
		t.Errorf("Mtime should be missing (%v) from repo 2 but it's %v", dummyTime, v)
	}

	repo1.UpdateMtime(file1, time1, time2)

	// Now it should return time2 only when time1 is passed as the argument

	if v := repo1.GetMtime(file1, time1); !v.Equal(time2) {
		t.Errorf("Mtime should be %v for disk time %v but we got %v", time2, time1, v)
	}

	if v := repo1.GetMtime(file1, dummyTime); !v.Equal(dummyTime) {
		t.Errorf("Mtime should be %v for disk time %v but we got %v", dummyTime, dummyTime, v)
	}

	// repo2 shouldn't know about this file

	if v := repo2.GetMtime(file1, time1); !v.Equal(time1) {
		t.Errorf("Mtime should be %v for disk time %v in repo 2 but we got %v", time1, time1, v)
	}

	repo1.DeleteMtime(file1)

	// Now it should be gone

	if v := repo1.GetMtime(file1, time1); !v.Equal(time1) {
		t.Errorf("Mtime should be %v for disk time %v but we got %v", time1, time1, v)
	}

	// Try again but with Drop()

	repo1.UpdateMtime(file1, time1, time2)
	repo1.Drop()

	if v := repo1.GetMtime(file1, time1); !v.Equal(time1) {
		t.Errorf("Mtime should be %v for disk time %v but we got %v", time1, time1, v)
	}
}
