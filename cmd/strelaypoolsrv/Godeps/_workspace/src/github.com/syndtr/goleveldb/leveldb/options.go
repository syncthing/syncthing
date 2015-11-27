// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func dupOptions(o *opt.Options) *opt.Options {
	newo := &opt.Options{}
	if o != nil {
		*newo = *o
	}
	if newo.Strict == 0 {
		newo.Strict = opt.DefaultStrict
	}
	return newo
}

func (s *session) setOptions(o *opt.Options) {
	no := dupOptions(o)
	// Alternative filters.
	if filters := o.GetAltFilters(); len(filters) > 0 {
		no.AltFilters = make([]filter.Filter, len(filters))
		for i, filter := range filters {
			no.AltFilters[i] = &iFilter{filter}
		}
	}
	// Comparer.
	s.icmp = &iComparer{o.GetComparer()}
	no.Comparer = s.icmp
	// Filter.
	if filter := o.GetFilter(); filter != nil {
		no.Filter = &iFilter{filter}
	}

	s.o = &cachedOptions{Options: no}
	s.o.cache()
}

type cachedOptions struct {
	*opt.Options

	compactionExpandLimit []int
	compactionGPOverlaps  []int
	compactionSourceLimit []int
	compactionTableSize   []int
	compactionTotalSize   []int64
}

func (co *cachedOptions) cache() {
	numLevel := co.Options.GetNumLevel()

	co.compactionExpandLimit = make([]int, numLevel)
	co.compactionGPOverlaps = make([]int, numLevel)
	co.compactionSourceLimit = make([]int, numLevel)
	co.compactionTableSize = make([]int, numLevel)
	co.compactionTotalSize = make([]int64, numLevel)

	for level := 0; level < numLevel; level++ {
		co.compactionExpandLimit[level] = co.Options.GetCompactionExpandLimit(level)
		co.compactionGPOverlaps[level] = co.Options.GetCompactionGPOverlaps(level)
		co.compactionSourceLimit[level] = co.Options.GetCompactionSourceLimit(level)
		co.compactionTableSize[level] = co.Options.GetCompactionTableSize(level)
		co.compactionTotalSize[level] = co.Options.GetCompactionTotalSize(level)
	}
}

func (co *cachedOptions) GetCompactionExpandLimit(level int) int {
	return co.compactionExpandLimit[level]
}

func (co *cachedOptions) GetCompactionGPOverlaps(level int) int {
	return co.compactionGPOverlaps[level]
}

func (co *cachedOptions) GetCompactionSourceLimit(level int) int {
	return co.compactionSourceLimit[level]
}

func (co *cachedOptions) GetCompactionTableSize(level int) int {
	return co.compactionTableSize[level]
}

func (co *cachedOptions) GetCompactionTotalSize(level int) int64 {
	return co.compactionTotalSize[level]
}
