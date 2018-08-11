package cuckoo

import (
	"encoding/binary"
	"sync"

	farm "github.com/dgryski/go-farm"
)

const magicNumber uint64 = 0x5bd1e995

// Filter ...
//
// Cuckoo filter type
type Filter struct {
	sync.Mutex
	buckets           []bucket
	bucketEntries     uint
	bucketTotal       uint
	capacity          uint
	count             uint
	fingerprintLength uint
	kicks             uint
}

// New ...
//
// Create a new Filter with an optional set of ConfigOption to configure settings.
//
// Example: New Filter with custom config option
//
// New(FingerprintLength(4))
//
// Example: New Filter with default config
//
// New()
//
// returns a Filter type
func New(opts ...ConfigOption) (filter *Filter) {
	filter = &Filter{}
	for _, option := range opts {
		option(filter)
	}
	filter.configureDefaults()
	filter.createBuckets()
	return
}

// Insert ...
//
// Add a new item of []byte to a Filter
//
// Example:
//
// filter.Insert([]byte("new-item"))
//
// returns a boolean of whether the item was inserted or not
func (f *Filter) Insert(item []byte) bool {
	fp := newFingerprint(item, f.fingerprintLength)
	i1 := uint(farm.Hash64(item)) % f.capacity
	i2 := f.alternateIndex(fp, i1)
	if f.insert(fp, i1) || f.insert(fp, i2) {
		return true
	}
	return f.relocationInsert(fp, i2)
}

// InsertUnique ...
//
// Add a new item of []byte to a Filter only if it doesn't already exist.
// Will do a Lookup of item first.
//
// Example:
//
// filter.InsertUnique([]byte("new-item"))
//
// returns a boolean of whether the item was inserted or not
func (f *Filter) InsertUnique(item []byte) bool {
	if f.Lookup(item) {
		return true
	}
	return f.Insert(item)
}

// Lookup ...
//
// Check if an item of []byte exists in the Filter
//
// Example:
//
// filter.Lookup([]byte("new-item"))
//
// returns a boolean of whether the item exists or not
func (f *Filter) Lookup(item []byte) bool {
	fp := newFingerprint(item, f.fingerprintLength)
	i1 := uint(farm.Hash64(item)) % f.capacity
	i2 := f.alternateIndex(fp, i1)
	if f.lookup(fp, i1) || f.lookup(fp, i2) {
		return true
	}
	return false
}

// Delete ...
//
// Delete an item of []byte if it exists in the Filter
//
// Example:
//
// filter.Delete([]byte("new-item"))
//
// returns a boolean of whether the item was deleted or not
func (f *Filter) Delete(item []byte) bool {
	fp := newFingerprint(item, f.fingerprintLength)
	i1 := uint(farm.Hash64(item)) % f.capacity
	i2 := f.alternateIndex(fp, i1)
	if f.delete(fp, i1) || f.delete(fp, i2) {
		return true
	}
	return false
}

// ItemCount ...
//
// Get an estimate of the total items in the Filter. Could be drastically off
// if using Insert with many duplicate items. To get a more accurate total
// using InsertUnique can be used
//
// Example:
//
// filter.ItemCount()
//
// returns an uint of the total items in the Filter
func (f *Filter) ItemCount() uint {
	return f.count
}

func (f *Filter) insert(fp fingerprint, idx uint) bool {
	f.Lock()
	defer f.Unlock()
	if f.buckets[idx].insert(fp) {
		f.count++
		return true
	}
	return false
}

func (f *Filter) relocationInsert(fp fingerprint, i uint) bool {
	f.Lock()
	defer f.Unlock()
	for k := uint(0); k < f.kicks; k++ {
		f.buckets[i].relocate(fp)
		i = f.alternateIndex(fp, i)
		if f.buckets[i].insert(fp) {
			f.count++
			return true
		}
	}
	return false
}

func (f *Filter) lookup(fp fingerprint, i uint) bool {
	if f.buckets[i].lookup(fp) {
		return true
	}
	return false
}

func (f *Filter) delete(fp fingerprint, idx uint) bool {
	if f.buckets[idx].delete(fp) {
		f.count--
		return true
	}
	return false
}

func (f *Filter) createBuckets() {
	buckets := make([]bucket, f.capacity, f.capacity)
	for i := range buckets {
		buckets[i] = make([]fingerprint, f.bucketEntries, f.bucketEntries)
	}
	f.buckets = buckets
}

func (f *Filter) alternateIndex(fp fingerprint, i uint) uint {
	bytes := make([]byte, 64, 64)
	for i, b := range fp {
		bytes[i] = b
	}

	hash := binary.LittleEndian.Uint64(bytes)
	return uint(uint64(i)^(hash*magicNumber)) % f.capacity
}
