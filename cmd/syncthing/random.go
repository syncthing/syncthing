// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"crypto/md5"
	cryptoRand "crypto/rand"
	"encoding/binary"
	"io"
	mathRand "math/rand"
)

// randomCharset contains the characters that can make up a randomString().
const randomCharset = "01234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-"

// predictableRandom is an RNG that will always have the same sequence. It
// will be seeded with the device ID during startup, so that the sequence is
// predictable but varies between instances.
var predictableRandom = mathRand.New(mathRand.NewSource(42))

func init() {
	// The default RNG should be seeded with something good.
	mathRand.Seed(randomInt64())
}

// randomString returns a string of random characters (taken from
// randomCharset) of the specified length.
func randomString(l int) string {
	bs := make([]byte, l)
	for i := range bs {
		bs[i] = randomCharset[mathRand.Intn(len(randomCharset))]
	}
	return string(bs)
}

// randomInt64 returns a strongly random int64, slowly
func randomInt64() int64 {
	var bs [8]byte
	_, err := io.ReadFull(cryptoRand.Reader, bs[:])
	if err != nil {
		panic("randomness failure: " + err.Error())
	}
	return seedFromBytes(bs[:])
}

// seedFromBytes calculates a weak 64 bit hash from the given byte slice,
// suitable for use a predictable random seed.
func seedFromBytes(bs []byte) int64 {
	h := md5.New()
	h.Write(bs)
	s := h.Sum(nil)
	// The MD5 hash of the byte slice is 16 bytes long. We interpret it as two
	// uint64s and XOR them together.
	return int64(binary.BigEndian.Uint64(s[0:]) ^ binary.BigEndian.Uint64(s[8:]))
}
