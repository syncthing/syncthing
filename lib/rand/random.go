// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package rand implements functions similar to math/rand in the standard
// library, but on top of a secure random number generator.
package rand

import (
	"io"
	mathRand "math/rand"
	"reflect"
)

// Reader is the standard crypto/rand.Reader with added buffering.
var Reader = defaultSecureSource

func Read(p []byte) (int, error) {
	return io.ReadFull(defaultSecureSource, p)
}

// randomCharset contains the characters that can make up a rand.String().
const randomCharset = "2345679abcdefghijkmnopqrstuvwxyzACDEFGHJKLMNPQRSTUVWXYZ"

var (
	// defaultSecureSource is a concurrency-safe, cryptographically secure
	// math/rand.Source.
	defaultSecureSource = newSecureSource()

	// defaultSecureRand is a math/rand.Rand based on the secure source.
	defaultSecureRand = mathRand.New(defaultSecureSource)
)

// String returns a cryptographically secure random string of characters
// (taken from randomCharset) of the specified length. The returned string
// contains ~5.8 bits of entropy per character, due to the character set used.
func String(l int) string {
	bs := make([]byte, l)
	for i := range bs {
		bs[i] = randomCharset[defaultSecureRand.Intn(len(randomCharset))]
	}
	return string(bs)
}

// Int63 returns a cryptographically secure random int63.
func Int63() int64 {
	return defaultSecureSource.Int63()
}

// Uint64 returns a cryptographically secure strongly random uint64.
func Uint64() uint64 {
	return defaultSecureSource.Uint64()
}

// Intn returns, as an int, a cryptographically secure non-negative
// random number in [0,n). It panics if n <= 0.
func Intn(n int) int {
	return defaultSecureRand.Intn(n)
}

// Shuffle the order of elements in slice.
func Shuffle(slice interface{}) {
	rv := reflect.ValueOf(slice)
	swap := reflect.Swapper(slice)
	length := rv.Len()
	if length < 2 {
		return
	}
	defaultSecureRand.Shuffle(length, swap)
}
