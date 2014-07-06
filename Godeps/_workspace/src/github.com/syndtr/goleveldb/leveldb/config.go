// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

const (
	kNumLevels = 7

	// Level-0 compaction is started when we hit this many files.
	kL0_CompactionTrigger float64 = 4

	// Soft limit on number of level-0 files.  We slow down writes at this point.
	kL0_SlowdownWritesTrigger = 8

	// Maximum number of level-0 files.  We stop writes at this point.
	kL0_StopWritesTrigger = 12

	// Maximum level to which a new compacted memdb is pushed if it
	// does not create overlap.  We try to push to level 2 to avoid the
	// relatively expensive level 0=>1 compactions and to avoid some
	// expensive manifest file operations.  We do not push all the way to
	// the largest level since that can generate a lot of wasted disk
	// space if the same key space is being repeatedly overwritten.
	kMaxMemCompactLevel = 2

	// Maximum size of a table.
	kMaxTableSize = 2 * 1048576

	// Maximum bytes of overlaps in grandparent (i.e., level+2) before we
	// stop building a single file in a level->level+1 compaction.
	kMaxGrandParentOverlapBytes = 10 * kMaxTableSize

	// Maximum number of bytes in all compacted files.  We avoid expanding
	// the lower level file set of a compaction if it would make the
	// total compaction cover more than this many bytes.
	kExpCompactionMaxBytes = 25 * kMaxTableSize
)
