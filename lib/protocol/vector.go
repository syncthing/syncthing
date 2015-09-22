// Copyright (C) 2015 The Protocol Authors.

package protocol

// The Vector type represents a version vector. The zero value is a usable
// version vector. The vector has slice semantics and some operations on it
// are "append-like" in that they may return the same vector modified, or a
// new allocated Vector with the modified contents.
type Vector []Counter

// Counter represents a single counter in the version vector.
type Counter struct {
	ID    uint64
	Value uint64
}

// Update returns a Vector with the index for the specific ID incremented by
// one. If it is possible, the vector v is updated and returned. If it is not,
// a copy will be created, updated and returned.
func (v Vector) Update(ID uint64) Vector {
	for i := range v {
		if v[i].ID == ID {
			// Update an existing index
			v[i].Value++
			return v
		} else if v[i].ID > ID {
			// Insert a new index
			nv := make(Vector, len(v)+1)
			copy(nv, v[:i])
			nv[i].ID = ID
			nv[i].Value = 1
			copy(nv[i+1:], v[i:])
			return nv
		}
	}
	// Append a new new index
	return append(v, Counter{ID, 1})
}

// Merge returns the vector containing the maximum indexes from a and b. If it
// is possible, the vector a is updated and returned. If it is not, a copy
// will be created, updated and returned.
func (a Vector) Merge(b Vector) Vector {
	var ai, bi int
	for bi < len(b) {
		if ai == len(a) {
			// We've reach the end of a, all that remains are appends
			return append(a, b[bi:]...)
		}

		if a[ai].ID > b[bi].ID {
			// The index from b should be inserted here
			n := make(Vector, len(a)+1)
			copy(n, a[:ai])
			n[ai] = b[bi]
			copy(n[ai+1:], a[ai:])
			a = n
		}

		if a[ai].ID == b[bi].ID {
			if v := b[bi].Value; v > a[ai].Value {
				a[ai].Value = v
			}
		}

		if bi < len(b) && a[ai].ID == b[bi].ID {
			bi++
		}
		ai++
	}

	return a
}

// Copy returns an identical vector that is not shared with v.
func (v Vector) Copy() Vector {
	nv := make(Vector, len(v))
	copy(nv, v)
	return nv
}

// Equal returns true when the two vectors are equivalent.
func (a Vector) Equal(b Vector) bool {
	return a.Compare(b) == Equal
}

// LesserEqual returns true when the two vectors are equivalent or a is Lesser
// than b.
func (a Vector) LesserEqual(b Vector) bool {
	comp := a.Compare(b)
	return comp == Lesser || comp == Equal
}

// LesserEqual returns true when the two vectors are equivalent or a is Greater
// than b.
func (a Vector) GreaterEqual(b Vector) bool {
	comp := a.Compare(b)
	return comp == Greater || comp == Equal
}

// Concurrent returns true when the two vectors are concrurrent.
func (a Vector) Concurrent(b Vector) bool {
	comp := a.Compare(b)
	return comp == ConcurrentGreater || comp == ConcurrentLesser
}

// Counter returns the current value of the given counter ID.
func (v Vector) Counter(id uint64) uint64 {
	for _, c := range v {
		if c.ID == id {
			return c.Value
		}
	}
	return 0
}
