// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package rand

import (
	crand "crypto/rand"
	mrand "math/rand/v2"
	"sync"
)

// The secureSource is a secure math/rand/v2.Source + io.Reader that is seeded
// from crypto/rand.Reader. It means we can use the convenience functions
// provided by math/rand/v2.Rand on top of a secure source of numbers. It is
// concurrency safe for ease of use.
type secureSource struct {
	cha mrand.ChaCha8
	mut sync.Mutex
}

func newSecureSource() *secureSource {
	s := new(secureSource)
	var seed [32]byte
	_, err := crand.Read(seed[:])
	if err != nil {
		panic("initializing crypto RNG: " + err.Error())
	}
	s.cha.Seed(seed)
	return s
}

func (*secureSource) Seed(int64) {
	panic("SecureSource is not seedable")
}

func (s *secureSource) Read(p []byte) (int, error) {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.cha.Read(p)
}

func (s *secureSource) Uint64() uint64 {
	s.mut.Lock()
	x := s.cha.Uint64()
	s.mut.Unlock()
	return x
}
