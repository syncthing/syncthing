package cuckoo

import (
	"sync"

	"github.com/DataDog/mmh3"
)

type fingerprint []byte

var hashSync sync.Mutex

func newFingerprint(item []byte, length uint) fingerprint {
	hashedFingerprint := calculateHash(item, length)
	fingerprinted := make(fingerprint, length, length)
	for i := uint(0); i < length; i++ {
		fingerprinted[i] = hashedFingerprint[i]
	}
	return fingerprinted
}

func calculateHash(item []byte, length uint) (hashedItem []byte) {
	hashedItem = mmh3.Hash128(item)
	return
}
