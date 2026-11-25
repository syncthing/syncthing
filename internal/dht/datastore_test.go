// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"testing"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/db/sqlite"
)

func TestDatastoreQuery(t *testing.T) {
	backend, err := sqlite.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		backend.Close()
	})

	ctx := t.Context()
	store := NewDatastore(db.NewTyped(backend, "dht"))
	for key, value := range map[string]string{
		"/records/first": "value",
		"/other/second":  "excluded",
	} {
		if err := store.Put(ctx, ds.NewKey(key), []byte(value)); err != nil {
			t.Fatal(err)
		}
	}

	results, err := store.Query(ctx, query.Query{Prefix: "/records"})
	if err != nil {
		t.Fatal(err)
	}
	defer results.Close()

	entries, err := results.Rest()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %v", len(entries), entries)
	}
	entry := entries[0]
	if entry.Key != "/records/first" || entry.Size != len("value") {
		t.Errorf("got key %q and size %d, want /records/first and size %d", entry.Key, entry.Size, len("value"))
	}
}
