// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"github.com/syndtr/goleveldb/leveldb/cache"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func (s *session) setOptions(o *opt.Options) {
	s.o = &opt.Options{}
	if o != nil {
		*s.o = *o
	}
	// Alternative filters.
	if filters := o.GetAltFilters(); len(filters) > 0 {
		s.o.AltFilters = make([]filter.Filter, len(filters))
		for i, filter := range filters {
			s.o.AltFilters[i] = &iFilter{filter}
		}
	}
	// Block cache.
	switch o.GetBlockCache() {
	case nil:
		s.o.BlockCache = cache.NewLRUCache(opt.DefaultBlockCacheSize)
	case opt.NoCache:
		s.o.BlockCache = nil
	}
	// Comparer.
	s.cmp = &iComparer{o.GetComparer()}
	s.o.Comparer = s.cmp
	// Filter.
	if filter := o.GetFilter(); filter != nil {
		s.o.Filter = &iFilter{filter}
	}
}
