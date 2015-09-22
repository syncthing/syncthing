// Copyright (C) 2015 The Protocol Authors.

package protocol

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
func (a Vector) Compare(b Vector) Ordering {
	var ai, bi int     // index into a and b
	var av, bv Counter // value at current index

	result := Equal

	for ai < len(a) || bi < len(b) {
		var aMissing, bMissing bool

		if ai < len(a) {
			av = a[ai]
		} else {
			av = Counter{}
			aMissing = true
		}

		if bi < len(b) {
			bv = b[bi]
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

		if ai < len(a) && (av.ID <= bv.ID || bMissing) {
			ai++
		}
		if bi < len(b) && (bv.ID <= av.ID || aMissing) {
			bi++
		}
	}

	return result
}
