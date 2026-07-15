// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package olddb

import (
	"bytes"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/syncthing/syncthing/internal/db/olddb/backend"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
)

func TestIterateDeviceStatistics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index-v0.14.0.db")
	ldb, err := leveldb.OpenFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	device1 := protocol.NewDeviceID([]byte("device1"))
	device2 := protocol.NewDeviceID([]byte("device2"))
	entries := map[string][]byte{
		device1.String() + "lastSeen":         []byte("last seen value"),
		device1.String() + "lastConnDuration": []byte("connection duration value"),
		device2.String() + "lastSeen":         []byte("other last seen value"),
		device2.String() + "lastConnDuration": []byte("other connection duration value"),
	}
	for key, value := range entries {
		if err := ldb.Put(append([]byte{KeyTypeDeviceStatistic}, key...), value, nil); err != nil {
			t.Fatal(err)
		}
	}
	// Malformed keys in the same namespace must not prevent valid statistics
	// from being migrated.
	if err := ldb.Put([]byte{KeyTypeDeviceStatistic, 'x'}, []byte("invalid"), nil); err != nil {
		t.Fatal(err)
	}
	if err := ldb.Put([]byte{KeyTypeFolderStatistic, 'x'}, []byte("other namespace"), nil); err != nil {
		t.Fatal(err)
	}
	if err := ldb.Close(); err != nil {
		t.Fatal(err)
	}

	be, err := backend.OpenLevelDBRO(path)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()
	ll, err := NewLowlevel(be)
	if err != nil {
		t.Fatal(err)
	}

	got := make(map[string][]byte)
	err = ll.IterateDeviceStatistics(func(device protocol.DeviceID, key string, value []byte) error {
		got[device.String()+key] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, entries) {
		t.Errorf("unexpected device statistics: got %v, want %v", got, entries)
	}

	// Returned values must not alias iterator-owned memory.
	for key, value := range got {
		if !bytes.Equal(value, entries[key]) {
			t.Errorf("value for %q changed after iteration: got %q, want %q", key, value, entries[key])
		}
	}
}
