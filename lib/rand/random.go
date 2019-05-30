// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package rand implements functions similar to math/rand in the standard
// library, but on top of a secure random number generator.
package rand

import (
	"crypto/md5"
	cryptoRand "crypto/rand"
	"encoding/binary"
	"io"
	mathRand "math/rand"
	"reflect"
)

// Reader is the standard crypto/rand.Reader, re-exported for convenience
var Reader = cryptoRand.Reader

// randomCharset contains the characters that can make up a randomString().
const randomCharset = "2345679abcdefghijkmnopqrstuvwxyzACDEFGHJKLMNPQRSTUVWXYZ"

var (
	// defaultSecureSource is a concurrency safe math/rand.Source with a
	// cryptographically sound base.
	defaltSecureSource = newSecureSource()

	// defaultSecureRand is a math/rand.Rand based on the secure source.
	defaultSecureRand = mathRand.New(defaltSecureSource)
)

// String returns a strongly random string of characters (taken from
// randomCharset) of the specified length. The returned string contains ~5.8
// bits of entropy per character, due to the character set used.
func String(l int) string {
	bs := make([]byte, l)
	for i := range bs {
		bs[i] = randomCharset[defaultSecureRand.Intn(len(randomCharset))]
	}
	return string(bs)
}

// Int63 returns a strongly random int63
func Int63() int64 {
	return defaltSecureSource.Int63()
}

// Int64 returns a strongly random int64
func Int64() int64 {
	var bs [8]byte
	_, err := io.ReadFull(cryptoRand.Reader, bs[:])
	if err != nil {
		panic("randomness failure: " + err.Error())
	}
	return int64(binary.BigEndian.Uint64(bs[:]))
}

// Intn returns, as an int, a non-negative strongly random number in [0,n).
// It panics if n <= 0.
func Intn(n int) int {
	return defaultSecureRand.Intn(n)
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

// Shuffle the order of elements
func Shuffle(slice interface{}) {
	rv := reflect.ValueOf(slice)
	swap := reflect.Swapper(slice)
	length := rv.Len()
	if length < 2 {
		return
	}
	defaultSecureRand.Shuffle(length, swap)
}
