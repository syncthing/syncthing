// Copyright (C) 2015 The Protocol Authors.

package protocol

// The Vector type represents a version vector. The zero value is a usable
// version vector. The vector has slice semantics and some operations on it
// are "append-like" in that they may return the same vector modified, or v
// new allocated Vector with the modified contents.
type Vector []Counter

// Counter represents a single counter in the version vector.
type Counter struct {
	ID    ShortID
	Value uint64
}

// Update returns a Vector with the index for the specific ID incremented by
// one. If it is possible, the vector v is updated and returned. If it is not,
// a copy will be created, updated and returned.
func (v Vector) Update(id ShortID) Vector {
	for i := range v {
		if v[i].ID == id {
			// Update an existing index
			v[i].Value++
			return v
		} else if v[i].ID > id {
			// Insert a new index
			nv := make(Vector, len(v)+1)
			copy(nv, v[:i])
			nv[i].ID = id
			nv[i].Value = 1
			copy(nv[i+1:], v[i:])
			return nv
		}
	}
	// Append a new index
	return append(v, Counter{id, 1})
}

// Merge returns the vector containing the maximum indexes from v and b. If it
// is possible, the vector v is updated and returned. If it is not, a copy
// will be created, updated and returned.
func (v Vector) Merge(b Vector) Vector {
	var vi, bi int
	for bi < len(b) {
		if vi == len(v) {
			// We've reach the end of v, all that remains are appends
			return append(v, b[bi:]...)
		}

		if v[vi].ID > b[bi].ID {
			// The index from b should be inserted here
			n := make(Vector, len(v)+1)
			copy(n, v[:vi])
			n[vi] = b[bi]
			copy(n[vi+1:], v[vi:])
			v = n
		}

		if v[vi].ID == b[bi].ID {
			if val := b[bi].Value; val > v[vi].Value {
				v[vi].Value = val
			}
		}

		if bi < len(b) && v[vi].ID == b[bi].ID {
			bi++
		}
		vi++
	}

	return v
}

// Copy returns an identical vector that is not shared with v.
func (v Vector) Copy() Vector {
	nv := make(Vector, len(v))
	copy(nv, v)
	return nv
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
	for _, c := range v {
		if c.ID == id {
			return c.Value
		}
	}
	return 0
}
