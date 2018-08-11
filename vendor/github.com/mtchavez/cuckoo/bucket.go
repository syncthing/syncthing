package cuckoo

import (
	"bytes"
	"math/rand"
)

type bucket []fingerprint

func (b bucket) insert(fp fingerprint) bool {
	for i, fprint := range b {
		if fprint == nil {
			b[i] = fp
			return true
		}
	}
	return false
}

func (b bucket) lookup(fp fingerprint) bool {
	for _, fprint := range b {
		if bytes.Equal(fp, fprint) {
			return true
		}
	}
	return false
}

func (b bucket) delete(fp fingerprint) bool {
	for i, fprint := range b {
		if bytes.Equal(fp, fprint) {
			b[i] = nil
			return true
		}
	}
	return false
}

func (b bucket) relocate(fp fingerprint) fingerprint {
	i := rand.Intn(len(b))
	b[i], fp = fp, b[i]

	return fp
}
