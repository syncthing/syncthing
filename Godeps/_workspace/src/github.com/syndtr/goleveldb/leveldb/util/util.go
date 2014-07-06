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
	ErrNotFound = errors.New("leveldb: not found")
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
	SetReleaser(releaser Releaser)
}

// BasicReleaser provides basic implementation of Releaser and ReleaseSetter.
type BasicReleaser struct {
	releaser Releaser
}

// Release implements Releaser.Release.
func (r *BasicReleaser) Release() {
	if r.releaser != nil {
		r.releaser.Release()
		r.releaser = nil
	}
}

// SetReleaser implements ReleaseSetter.SetReleaser.
func (r *BasicReleaser) SetReleaser(releaser Releaser) {
	r.releaser = releaser
}
