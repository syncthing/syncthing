// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import "syscall"

func setControlMessage(fd int, opt *rawOpt, cf ControlFlags, on bool) error {
	// TODO(mikio): Implement this
	return syscall.EPLAN9
}

func newControlMessage(opt *rawOpt) (oob []byte) {
	// TODO(mikio): Implement this
	return nil
}

func parseControlMessage(b []byte) (*ControlMessage, error) {
	// TODO(mikio): Implement this
	return nil, syscall.EPLAN9
}

func marshalControlMessage(cm *ControlMessage) (oob []byte) {
	// TODO(mikio): Implement this
	return nil
}
