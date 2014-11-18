// Copyright (c) 2013, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package util provides utilities used throughout leveldb.
package util

import (
	"errors"
)

var (
	ErrReleased    = errors.New("leveldb: resource already relesed")
	ErrHasReleaser = errors.New("leveldb: releaser already defined")
)

// Releaser is the interface that wraps the basic Release method.
type Releaser interface {
	// Release releases associated resources. Release should always success
	// and can be called multipe times without causing error.
	Release()
}

// ReleaseSetter is the interface that wraps the basic SetReleaser method.
type ReleaseSetter interface {
	// SetReleaser associates the given releaser to the resources. The
	// releaser will be called once coresponding resources released.
	// Calling SetReleaser with nil will clear the releaser.
	//
	// This will panic if a releaser already present or coresponding
	// resource is already released. Releaser should be cleared first
	// before assigned a new one.
	SetReleaser(releaser Releaser)
}

// BasicReleaser provides basic implementation of Releaser and ReleaseSetter.
type BasicReleaser struct {
	releaser Releaser
	released bool
}

// Released returns whether Release method already called.
func (r *BasicReleaser) Released() bool {
	return r.released
}

// Release implements Releaser.Release.
func (r *BasicReleaser) Release() {
	if !r.released {
		if r.releaser != nil {
			r.releaser.Release()
			r.releaser = nil
		}
		r.released = true
	}
}

// SetReleaser implements ReleaseSetter.SetReleaser.
func (r *BasicReleaser) SetReleaser(releaser Releaser) {
	if r.released {
		panic(ErrReleased)
	}
	if r.releaser != nil && releaser != nil {
		panic(ErrHasReleaser)
	}
	r.releaser = releaser
}

type NoopReleaser struct{}

func (NoopReleaser) Release() {}
