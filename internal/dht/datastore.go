// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"context"
	"iter"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/syncthing/syncthing/internal/db"
)

// datastore provides database-backed storage for the DHT values and
// routing table.
type datastore struct {
	store *db.Typed
}

// NewDatastore provides database-backed storage for DHT values or routing
// data.
func NewDatastore(store *db.Typed) ds.Batching {
	return &datastore{store: store}
}

func (s *datastore) Put(ctx context.Context, key ds.Key, value []byte) error {
	result := "success"
	err := s.store.PutBytes(key.String(), value)
	if err != nil {
		result = "error"
	}
	metricDatastoreOperationsTotal.WithLabelValues(s.store.Prefix(), "put", result).Inc()
	return err
}

func (s *datastore) Get(ctx context.Context, key ds.Key) ([]byte, error) {
	value, ok, err := s.store.Bytes(key.String())
	if err != nil {
		metricDatastoreOperationsTotal.WithLabelValues(s.store.Prefix(), "get", "error").Inc()
		return nil, err
	}
	if !ok {
		metricDatastoreOperationsTotal.WithLabelValues(s.store.Prefix(), "get", "notfound").Inc()
		return nil, ds.ErrNotFound
	}
	metricDatastoreOperationsTotal.WithLabelValues(s.store.Prefix(), "get", "success").Inc()
	return value, nil
}

func (s *datastore) Has(ctx context.Context, key ds.Key) (bool, error) {
	return ds.GetBackedHas(ctx, s, key)
}

func (s *datastore) GetSize(ctx context.Context, key ds.Key) (int, error) {
	return ds.GetBackedSize(ctx, s, key)
}

func (s *datastore) Delete(ctx context.Context, key ds.Key) error {
	result := "success"
	err := s.store.Delete(key.String())
	if err != nil {
		result = "error"
	}
	metricDatastoreOperationsTotal.WithLabelValues(s.store.Prefix(), "delete", result).Inc()
	return err
}

func (s *datastore) Query(ctx context.Context, q query.Query) (query.Results, error) {
	prefix := s.store.Prefix()
	if err := ctx.Err(); err != nil {
		metricDatastoreOperationsTotal.WithLabelValues(prefix, "query", "error").Inc()
		return nil, err
	}

	next, stop := iter.Pull(func(yield func(query.Result) bool) {
		count := 0
		it, scanErr := s.store.KeyValues()
		for kv := range it {
			if ctx.Err() != nil {
				break
			}
			entry := query.Entry{Key: kv.Key, Size: len(kv.Value)}
			if !q.KeysOnly {
				entry.Value = kv.Value
			}
			count++
			if !yield(query.Result{Entry: entry}) {
				return
			}
		}
		err := ctx.Err()
		if err == nil {
			err = scanErr()
		}
		if err != nil {
			metricDatastoreOperationsTotal.WithLabelValues(prefix, "query", "error").Inc()
			yield(query.Result{Error: err})
			return
		}
		metricDatastoreOperationsTotal.WithLabelValues(prefix, "query", "success").Inc()
		metricDatastoreItems.WithLabelValues(prefix).Set(float64(count))
	})

	results := query.ResultsFromIterator(q, query.Iterator{
		Next: next,
		Close: func() error {
			stop()
			return nil
		},
	})

	return query.NaiveQueryApply(q, results), nil
}

func (s *datastore) Sync(ctx context.Context, prefix ds.Key) error {
	return nil
}

func (s *datastore) Close() error {
	return nil
}

func (s *datastore) Batch(context.Context) (ds.Batch, error) {
	return ds.NewBasicBatch(s), nil
}
