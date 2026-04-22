// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/syncthing/syncthing/internal/gen/bep"
)

// The Vector type represents a version vector. The zero value is a usable
// version vector. The vector has slice semantics and some operations on it
// are "append-like" in that they may return the same vector modified, or v
// new allocated Vector with the modified contents.
type Vector struct {
	Counters []Counter
}

func (v *Vector) String() string {
	var buf strings.Builder
	for i, c := range v.Counters {
		if i > 0 {
			buf.WriteRune(',')
		}
		var idbs [8]byte
		binary.BigEndian.PutUint64(idbs[:], uint64(c.ID))
		fmt.Fprintf(&buf, "%x:%d", idbs, c.Value)
	}
	return buf.String()
}

func (v *Vector) HumanString() string {
	var buf strings.Builder
	for i, c := range v.Counters {
		if i > 0 {
			buf.WriteRune(',')
		}
		fmt.Fprintf(&buf, "%s:%d", c.ID, c.Value)
	}
	return buf.String()
}

func (v *Vector) ToWire() *bep.Vector {
	counters := make([]*bep.Counter, len(v.Counters))
	for i, c := range v.Counters {
		counters[i] = c.toWire()
	}
	return &bep.Vector{
		Counters: counters,
	}
}

func VectorFromWire(w *bep.Vector) Vector {
	var v Vector
	if w == nil || len(w.Counters) == 0 {
		return v
	}
	v.Counters = make([]Counter, len(w.Counters))
	for i, c := range w.Counters {
		v.Counters[i] = counterFromWire(c)
	}
	return v
}

func VectorFromString(s string) (Vector, error) {
	pairs := strings.Split(s, ",")
	var v Vector
	v.Counters = make([]Counter, len(pairs))
	for i, pair := range pairs {
		idStr, valStr, ok := strings.Cut(pair, ":")
		if !ok {
			return Vector{}, fmt.Errorf("bad pair %q", pair)
		}
		idslice, err := hex.DecodeString(idStr)
		if err != nil {
			return Vector{}, fmt.Errorf("bad id in pair %q", pair)
		}
		var idbs [8]byte
		copy(idbs[8-len(idslice):], idslice)
		id := binary.BigEndian.Uint64(idbs[:])
		val, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			return Vector{}, fmt.Errorf("bad val in pair %q", pair)
		}
		v.Counters[i] = Counter{ID: ShortID(id), Value: val}
	}
	return v, nil
}

// Counter represents a single counter in the version vector.
type Counter struct {
	ID    ShortID
	Value uint64
}

func (c *Counter) toWire() *bep.Counter {
	return &bep.Counter{
		Id:    uint64(c.ID),
		Value: c.Value,
	}
}

func counterFromWire(w *bep.Counter) Counter {
	return Counter{
		ID:    ShortID(w.Id),
		Value: w.Value,
	}
}

// Update returns a Vector with the index for the specific ID incremented by
// one. If it is possible, the vector v is updated and returned. If it is not,
// a copy will be created, updated and returned.
func (v Vector) Update(id ShortID) Vector {
	now := uint64(time.Now().Unix())
	return v.updateWithNow(id, now)
}

func (v Vector) updateWithNow(id ShortID, now uint64) Vector {
	for i := range v.Counters {
		if v.Counters[i].ID == id {
			// Update an existing index
			v.Counters[i].Value = max(v.Counters[i].Value+1, now)
			return v
		} else if v.Counters[i].ID > id {
			// Insert a new index
			nv := make([]Counter, len(v.Counters)+1)
			copy(nv, v.Counters[:i])
			nv[i].ID = id
			nv[i].Value = max(1, now)
			copy(nv[i+1:], v.Counters[i:])
			return Vector{Counters: nv}
		}
	}

	// Append a new index
	return Vector{Counters: append(v.Counters, Counter{
		ID:    id,
		Value: max(1, now),
	})}
}

// Merge returns the vector containing the maximum indexes from v and b. If it
// is possible, the vector v is updated and returned. If it is not, a copy
// will be created, updated and returned.
func (v Vector) Merge(b Vector) Vector {
	var vi, bi int
	for bi < len(b.Counters) {
		if vi == len(v.Counters) {
			// We've reach the end of v, all that remains are appends
			return Vector{Counters: append(v.Counters, b.Counters[bi:]...)}
		}

		if v.Counters[vi].ID > b.Counters[bi].ID {
			// The index from b should be inserted here
			n := make([]Counter, len(v.Counters)+1)
			copy(n, v.Counters[:vi])
			n[vi] = b.Counters[bi]
			copy(n[vi+1:], v.Counters[vi:])
			v.Counters = n
		}

		if v.Counters[vi].ID == b.Counters[bi].ID {
			if val := b.Counters[bi].Value; val > v.Counters[vi].Value {
				v.Counters[vi].Value = val
			}
		}

		if bi < len(b.Counters) && v.Counters[vi].ID == b.Counters[bi].ID {
			bi++
		}
		vi++
	}

	return v
}

// Copy returns an identical vector that is not shared with v.
func (v Vector) Copy() Vector {
	nv := make([]Counter, len(v.Counters))
	copy(nv, v.Counters)
	return Vector{Counters: nv}
}

// Equal returns true when the two vectors are equivalent.
func (v Vector) Equal(b Vector) bool {
	return v.Compare(b) == Equal
}

// LesserEqual returns true when the two vectors are equivalent or v is Lesser
// than b.
func (v Vector) LesserEqual(b Vector) bool {
	comp := v.Compare(b)
	return comp == Lesser || comp == Equal
}

// GreaterEqual returns true when the two vectors are equivalent or v is Greater
// than b.
func (v Vector) GreaterEqual(b Vector) bool {
	comp := v.Compare(b)
	return comp == Greater || comp == Equal
}

// Concurrent returns true when the two vectors are concurrent.
func (v Vector) Concurrent(b Vector) bool {
	comp := v.Compare(b)
	return comp == ConcurrentGreater || comp == ConcurrentLesser
}

// Counter returns the current value of the given counter ID.
func (v Vector) Counter(id ShortID) uint64 {
	for _, c := range v.Counters {
		if c.ID == id {
			return c.Value
		}
	}
	return 0
}

// IsEmpty returns true when there are no counters.
func (v Vector) IsEmpty() bool {
	return len(v.Counters) == 0
}

// DropOthers removes all counters, keeping only the one with given id. If there
// is no such counter, an empty Vector is returned.
func (v Vector) DropOthers(id ShortID) Vector {
	for i, c := range v.Counters {
		if c.ID == id {
			v.Counters = v.Counters[i : i+1]
			return v
		}
	}
	return Vector{}
}

// Ordering represents the relationship between two Vectors.
type Ordering int

const (
	Equal Ordering = iota
	Greater
	Lesser
	ConcurrentLesser
	ConcurrentGreater
)

// There's really no such thing as "concurrent lesser" and "concurrent
// greater" in version vectors, just "concurrent". But it's useful to be able
// to get a strict ordering between versions for stable sorts and so on, so we
// return both variants. The convenience method Concurrent() can be used to
// check for either case.

// Compare returns the Ordering that describes a's relation to b.
func (v Vector) Compare(b Vector) Ordering {
	var ai, bi int     // index into a and b
	var av, bv Counter // value at current index

	result := Equal

	for ai < len(v.Counters) || bi < len(b.Counters) {
		var aMissing, bMissing bool

		if ai < len(v.Counters) {
			av = v.Counters[ai]
		} else {
			av = Counter{}
			aMissing = true
		}

		if bi < len(b.Counters) {
			bv = b.Counters[bi]
		} else {
			bv = Counter{}
			bMissing = true
		}

		switch {
		case av.ID == bv.ID:
			// We have a counter value for each side
			if av.Value > bv.Value {
				if result == Lesser {
					return ConcurrentLesser
				}
				result = Greater
			} else if av.Value < bv.Value {
				if result == Greater {
					return ConcurrentGreater
				}
				result = Lesser
			}

		case !aMissing && av.ID < bv.ID || bMissing:
			// Value is missing on the b side
			if av.Value > 0 {
				if result == Lesser {
					return ConcurrentLesser
				}
				result = Greater
			}

		case !bMissing && bv.ID < av.ID || aMissing:
			// Value is missing on the a side
			if bv.Value > 0 {
				if result == Greater {
					return ConcurrentGreater
				}
				result = Lesser
			}
		}

		if ai < len(v.Counters) && (av.ID <= bv.ID || bMissing) {
			ai++
		}
		if bi < len(b.Counters) && (bv.ID <= av.ID || aMissing) {
			bi++
		}
	}

	return result
}
