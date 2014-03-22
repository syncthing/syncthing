// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import "syscall"

func ipv6PathMTU(fd int) (int, error) {
	// TODO(mikio): Implement this
	return 0, syscall.EPLAN9
}
