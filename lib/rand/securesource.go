// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package rand

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"io"
	"sync"
)

// The secureSource is a math/rand.Source + io.Reader that reads bytes from
// crypto/rand.Reader. It means we can use the convenience functions
// provided by math/rand.Rand on top of a secure source of numbers. It is
// concurrency safe for ease of use.
type secureSource struct {
	rd  io.Reader
	mut sync.Mutex
	buf [8]byte
}

func newSecureSource() *secureSource {
	return &secureSource{
		// Using buffering on top of the rand.Reader increases our
		// performance by about 20%, even though it means we must use
		// locking.
		rd: bufio.NewReader(rand.Reader),
	}
}

func (*secureSource) Seed(int64) {
	panic("SecureSource is not seedable")
}

func (s *secureSource) Int63() int64 {
	return int64(s.Uint64() & (1<<63 - 1))
}

func (s *secureSource) Read(p []byte) (int, error) {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.rd.Read(p)
}

func (s *secureSource) Uint64() uint64 {
	// Read eight bytes of entropy from the buffered, secure random number
	// generator. The buffered reader isn't concurrency safe, so we lock
	// around that.
	s.mut.Lock()
	defer s.mut.Unlock()

	_, err := io.ReadFull(s.rd, s.buf[:])
	if err != nil {
		panic("randomness failure: " + err.Error())
	}
	return binary.LittleEndian.Uint64(s.buf[:])
}
