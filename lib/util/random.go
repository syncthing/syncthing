// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package util

import (
	"crypto/md5"
	cryptoRand "crypto/rand"
	"encoding/binary"
	"io"
	mathRand "math/rand"
)

// randomCharset contains the characters that can make up a randomString().
const randomCharset = "2345679abcdefghijkmnopqrstuvwxyzACDEFGHJKLMNPQRSTUVWXYZ"

func init() {
	// The default RNG should be seeded with something good.
	mathRand.Seed(RandomInt64())
}

// RandomString returns a string of random characters (taken from
// randomCharset) of the specified length.
func RandomString(l int) string {
	bs := make([]byte, l)
	for i := range bs {
		bs[i] = randomCharset[mathRand.Intn(len(randomCharset))]
	}
	return string(bs)
}

// RandomInt64 returns a strongly random int64, slowly
func RandomInt64() int64 {
	var bs [8]byte
	_, err := io.ReadFull(cryptoRand.Reader, bs[:])
	if err != nil {
		panic("randomness failure: " + err.Error())
	}
	return SeedFromBytes(bs[:])
}

// SeedFromBytes calculates a weak 64 bit hash from the given byte slice,
// suitable for use a predictable random seed.
func SeedFromBytes(bs []byte) int64 {
	h := md5.New()
	h.Write(bs)
	s := h.Sum(nil)
	// The MD5 hash of the byte slice is 16 bytes long. We interpret it as two
	// uint64s and XOR them together.
	return int64(binary.BigEndian.Uint64(s[0:]) ^ binary.BigEndian.Uint64(s[8:]))
}
