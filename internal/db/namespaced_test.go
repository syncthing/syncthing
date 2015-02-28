// Copyright (C) 2014 The Syncthing Authors.
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

package db

import (
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

func TestNamespacedInt(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	n1 := NewNamespacedKV(ldb, "foo")
	n2 := NewNamespacedKV(ldb, "bar")

	// Key is missing to start with

	if v, ok := n1.Int64("test"); v != 0 || ok {
		t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
	}

	n1.PutInt64("test", 42)

	// It should now exist in n1

	if v, ok := n1.Int64("test"); v != 42 || !ok {
		t.Errorf("Incorrect return v %v != 42 || ok %v != true", v, ok)
	}

	// ... but not in n2, which is in a different namespace

	if v, ok := n2.Int64("test"); v != 0 || ok {
		t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
	}

	n1.Delete("test")

	// It should no longer exist

	if v, ok := n1.Int64("test"); v != 0 || ok {
		t.Errorf("Incorrect return v %v != 0 || ok %v != false", v, ok)
	}
}

func TestNamespacedTime(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	n1 := NewNamespacedKV(ldb, "foo")

	if v, ok := n1.Time("test"); v != (time.Time{}) || ok {
		t.Errorf("Incorrect return v %v != %v || ok %v != false", v, time.Time{}, ok)
	}

	now := time.Now()
	n1.PutTime("test", now)

	if v, ok := n1.Time("test"); v != now || !ok {
		t.Errorf("Incorrect return v %v != %v || ok %v != true", v, now, ok)
	}
}

func TestNamespacedString(t *testing.T) {
	ldb, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}

	n1 := NewNamespacedKV(ldb, "foo")

	if v, ok := n1.String("test"); v != "" || ok {
		t.Errorf("Incorrect return v %q != \"\" || ok %v != false", v, ok)
	}

	n1.PutString("test", "yo")

	if v, ok := n1.String("test"); v != "yo" || !ok {
		t.Errorf("Incorrect return v %q != \"yo\" || ok %v != true", v, ok)
	}
}
