// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build !windows,!darwin

package files

import "code.google.com/p/go.text/unicode/norm"

func normalizedFilename(s string) string {
	return norm.NFC.String(s)
}

func nativeFilename(s string) string {
	return s
}
