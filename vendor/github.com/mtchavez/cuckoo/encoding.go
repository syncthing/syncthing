package cuckoo

import (
	"bytes"
	"encoding/gob"
	"os"
)

// A filter wrapper with exported fields used for marshalling
type encodedFilter struct {
	Buckets           []bucket
	BucketEntries     uint
	BucketTotal       uint
	Capacity          uint
	Count             uint
	FingerprintLength uint
	Kicks             uint
}

// MarshalBinary used to interact with gob encoding interface
func (f *Filter) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	ef := encodedFilter{
		Buckets:           f.buckets,
		BucketEntries:     f.bucketEntries,
		BucketTotal:       f.bucketTotal,
		Capacity:          f.capacity,
		Count:             f.count,
		FingerprintLength: f.fingerprintLength,
		Kicks:             f.kicks,
	}
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(ef); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary modifies the receiver so it must take a pointer receiver.
// Used to interact with gob encoding interface
func (f *Filter) UnmarshalBinary(data []byte) error {
	ef := encodedFilter{}
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(&ef); err != nil {
		return err
	}
	f.buckets = ef.Buckets
	f.bucketEntries = ef.BucketEntries
	f.bucketTotal = ef.BucketTotal
	f.capacity = ef.Capacity
	f.count = ef.Count
	f.fingerprintLength = ef.FingerprintLength
	f.kicks = ef.Kicks
	return nil
}

// Save takes a path to a file to save an encoded filter to disk
func (f *Filter) Save(path string) error {
	file, err := os.Create(path)
	defer file.Close()
	if err == nil {
		encoder := gob.NewEncoder(file)
		encoder.Encode(f)
	}
	return err
}

// Load takes a path to a file of an encoded Filter to load into memory
func Load(path string) (*Filter, error) {
	f := &Filter{}
	file, err := os.Open(path)
	defer file.Close()
	if err == nil {
		decoder := gob.NewDecoder(file)
		err = decoder.Decode(&f)
	}
	return f, err
}
