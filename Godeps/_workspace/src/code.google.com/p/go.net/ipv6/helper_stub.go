// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build dragonfly plan9 solaris

package ipv6

func (c *genericOpt) sysfd() (int, error) {
	// TODO(mikio): Implement this
	return 0, errOpNoSupport
}

func (c *dgramOpt) sysfd() (int, error) {
	// TODO(mikio): Implement this
	return 0, errOpNoSupport
}

func (c *payloadHandler) sysfd() (int, error) {
	// TODO(mikio): Implement this
	return 0, errOpNoSupport
}
