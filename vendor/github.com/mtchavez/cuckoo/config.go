package cuckoo

// ConfigOption ...
//
// Composed Filter function type used for configuring a Filter
type ConfigOption func(*Filter)

// Cuckoo Filter notation
// e target false positive rate
// f fingerprint length in bits
// α load factor (0 ≤ α ≤ 1)
// b number of entries per bucket
// m number of buckets
// n number of items
// C average bits per item
const (
	// Entries per bucket (b)
	defaultBucketEntries uint = 24
	// Bucket total (m) defaults to approx. 4 million
	defaultBucketTotal uint = 1 << 22
	// Length of fingreprint (f) set to log(n/b) ~6 bits
	defaultFingerprintLength uint = 6
	// Default attempts to find empty slot on insert
	defaultKicks uint = 500
)

// BucketEntries ...
//
// Number of entries per bucket denoted as b
//
// Example:
//
// New(BucketEntries(uint(42)))
func BucketEntries(entries uint) ConfigOption {
	return func(f *Filter) {
		f.bucketEntries = entries
	}
}

// BucketTotal ...
//
// Number of buckets in the Filter denoted as m
//
// Example:
//
// New(BucketTotal(uint(42)))
func BucketTotal(total uint) ConfigOption {
	return func(f *Filter) {
		f.bucketTotal = total
	}
}

// FingerprintLength ...
//
// Length of the item fingerprint denoted as f
//
// Example:
//
// New(FingerprintLength(uint(4)))
func FingerprintLength(length uint) ConfigOption {
	return func(f *Filter) {
		if length > uint(16) {
			length = uint(16)
		}
		f.fingerprintLength = length
	}
}

// Kicks ...
//
// Maximum number of kicks to attempt when bucket collisions occur
//
// Example:
//
// New(Kicks(uint(200)))
func Kicks(kicks uint) ConfigOption {
	return func(f *Filter) {
		f.kicks = kicks
	}
}

func capacity() ConfigOption {
	return func(f *Filter) {
		f.capacity = nextPowerOf2(uint64(f.bucketTotal)) / f.bucketEntries
		if f.capacity <= 0 {
			f.capacity = 1
		}
	}
}

func (f *Filter) configureDefaults() {
	if f.bucketTotal <= 0 {
		BucketTotal(defaultBucketTotal)(f)
	}

	if f.bucketEntries <= 0 {
		BucketEntries(defaultBucketEntries)(f)
	}

	if f.fingerprintLength <= 0 {
		FingerprintLength(defaultFingerprintLength)(f)
	}

	if f.kicks <= 0 {
		Kicks(defaultKicks)(f)
	}

	if f.capacity < 1 {
		capacity()(f)
	}
}

func nextPowerOf2(n uint64) uint {
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	n++
	return uint(n)
}
