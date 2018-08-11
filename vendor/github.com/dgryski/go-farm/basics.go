package farm

// Some primes between 2^63 and 2^64 for various uses.
const k0 uint64 = 0xc3a5c85c97cb3127
const k1 uint64 = 0xb492b66fbe98f273
const k2 uint64 = 0x9ae16a3b2f90404f

// Magic numbers for 32-bit hashing.  Copied from Murmur3.
const c1 uint32 = 0xcc9e2d51
const c2 uint32 = 0x1b873593

// A 32-bit to 32-bit integer hash copied from Murmur3.
func fmix(h uint32) uint32 {
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return h
}

func mur(a, h uint32) uint32 {
	// Helper from Murmur3 for combining two 32-bit values.
	a *= c1
	a = rotate32(a, 17)
	a *= c2
	h ^= a
	h = rotate32(h, 19)
	return h*5 + 0xe6546b64
}
