// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kcp

import (
	"runtime"
	"unsafe"
)

const wordSize = int(unsafe.Sizeof(uintptr(0)))
const supportsUnaligned = runtime.GOARCH == "386" || runtime.GOARCH == "amd64" || runtime.GOARCH == "ppc64" || runtime.GOARCH == "ppc64le" || runtime.GOARCH == "s390x"

// fastXORBytes xors in bulk. It only works on architectures that
// support unaligned read/writes.
func fastXORBytes(dst, a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}

	w := n / wordSize
	if w > 0 {
		wordBytes := w * wordSize
		fastXORWords(dst[:wordBytes], a[:wordBytes], b[:wordBytes])
	}

	for i := (n - n%wordSize); i < n; i++ {
		dst[i] = a[i] ^ b[i]
	}

	return n
}

func safeXORBytes(dst, a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	ex := n % 8
	for i := 0; i < ex; i++ {
		dst[i] = a[i] ^ b[i]
	}

	for i := ex; i < n; i += 8 {
		dst[i] = a[i] ^ b[i]
		dst[i+1] = a[i+1] ^ b[i+1]
		dst[i+2] = a[i+2] ^ b[i+2]
		dst[i+3] = a[i+3] ^ b[i+3]

		dst[i+4] = a[i+4] ^ b[i+4]
		dst[i+5] = a[i+5] ^ b[i+5]
		dst[i+6] = a[i+6] ^ b[i+6]
		dst[i+7] = a[i+7] ^ b[i+7]
	}
	return n
}

// xorBytes xors the bytes in a and b. The destination is assumed to have enough
// space. Returns the number of bytes xor'd.
func xorBytes(dst, a, b []byte) int {
	if supportsUnaligned {
		return fastXORBytes(dst, a, b)
	} else {
		// TODO(hanwen): if (dst, a, b) have common alignment
		// we could still try fastXORBytes. It is not clear
		// how often this happens, and it's only worth it if
		// the block encryption itself is hardware
		// accelerated.
		return safeXORBytes(dst, a, b)
	}
}

// fastXORWords XORs multiples of 4 or 8 bytes (depending on architecture.)
// The arguments are assumed to be of equal length.
func fastXORWords(dst, a, b []byte) {
	dw := *(*[]uintptr)(unsafe.Pointer(&dst))
	aw := *(*[]uintptr)(unsafe.Pointer(&a))
	bw := *(*[]uintptr)(unsafe.Pointer(&b))
	n := len(b) / wordSize
	ex := n % 8
	for i := 0; i < ex; i++ {
		dw[i] = aw[i] ^ bw[i]
	}

	for i := ex; i < n; i += 8 {
		dw[i] = aw[i] ^ bw[i]
		dw[i+1] = aw[i+1] ^ bw[i+1]
		dw[i+2] = aw[i+2] ^ bw[i+2]
		dw[i+3] = aw[i+3] ^ bw[i+3]
		dw[i+4] = aw[i+4] ^ bw[i+4]
		dw[i+5] = aw[i+5] ^ bw[i+5]
		dw[i+6] = aw[i+6] ^ bw[i+6]
		dw[i+7] = aw[i+7] ^ bw[i+7]
	}
}

func xorWords(dst, a, b []byte) {
	if supportsUnaligned {
		fastXORWords(dst, a, b)
	} else {
		safeXORBytes(dst, a, b)
	}
}
