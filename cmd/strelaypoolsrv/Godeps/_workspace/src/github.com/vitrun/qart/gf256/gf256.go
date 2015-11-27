// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gf256 implements arithmetic over the Galois Field GF(256).
package gf256

import "strconv"

// A Field represents an instance of GF(256) defined by a specific polynomial.
type Field struct {
	log [256]byte // log[0] is unused
	exp [510]byte
}

// NewField returns a new field corresponding to the polynomial poly
// and generator α.  The Reed-Solomon encoding in QR codes uses
// polynomial 0x11d with generator 2.
//
// The choice of generator α only affects the Exp and Log operations.
func NewField(poly, α int) *Field {
	if poly < 0x100 || poly >= 0x200 || reducible(poly) {
		panic("gf256: invalid polynomial: " + strconv.Itoa(poly))
	}

	var f Field
	x := 1
	for i := 0; i < 255; i++ {
		if x == 1 && i != 0 {
			panic("gf256: invalid generator " + strconv.Itoa(α) +
					" for polynomial " + strconv.Itoa(poly))
		}
		f.exp[i] = byte(x)
		f.exp[i+255] = byte(x)
		f.log[x] = byte(i)
		x = mul(x, α, poly)
	}
	f.log[0] = 255
	for i := 0; i < 255; i++ {
		if f.log[f.exp[i]] != byte(i) {
			panic("bad log")
		}
		if f.log[f.exp[i+255]] != byte(i) {
			panic("bad log")
		}
	}
	for i := 1; i < 256; i++ {
		if f.exp[f.log[i]] != byte(i) {
			panic("bad log")
		}
	}

	return &f
}

// nbit returns the number of significant in p.
func nbit(p int) uint {
	n := uint(0)
	for ; p > 0; p >>= 1 {
		n++
	}
	return n
}

// polyDiv divides the polynomial p by q and returns the remainder.
func polyDiv(p, q int) int {
	np := nbit(p)
	nq := nbit(q)
	for ; np >= nq; np-- {
		if p&(1<<(np-1)) != 0 {
			p ^= q << (np - nq)
		}
	}
	return p
}

// mul returns the product x*y mod poly, a GF(256) multiplication.
func mul(x, y, poly int) int {
	z := 0
	for x > 0 {
		if x&1 != 0 {
			z ^= y
		}
		x >>= 1
		y <<= 1
		if y&0x100 != 0 {
			y ^= poly
		}
	}
	return z
}

// reducible reports whether p is reducible.
func reducible(p int) bool {
	// Multiplying n-bit * n-bit produces (2n-1)-bit,
	// so if p is reducible, one of its factors must be
	// of np/2+1 bits or fewer.
	np := nbit(p)
	for q := 2; q < int(1<<(np/2+1)); q++ {
		if polyDiv(p, q) == 0 {
			return true
		}
	}
	return false
}

// Add returns the sum of x and y in the field.
func (f *Field) Add(x, y byte) byte {
	return x ^ y
}

// Exp returns the the base-α exponential of e in the field.
// If e < 0, Exp returns 0.
func (f *Field) Exp(e int) byte {
	if e < 0 {
		return 0
	}
	return f.exp[e%255]
}

// Log returns the base-α logarithm of x in the field.
// If x == 0, Log returns -1.
func (f *Field) Log(x byte) int {
	if x == 0 {
		return -1
	}
	return int(f.log[x])
}

// Inv returns the multiplicative inverse of x in the field.
// If x == 0, Inv returns 0.
func (f *Field) Inv(x byte) byte {
	if x == 0 {
		return 0
	}
	return f.exp[255-f.log[x]]
}

// Mul returns the product of x and y in the field.
func (f *Field) Mul(x, y byte) byte {
	if x == 0 || y == 0 {
		return 0
	}
	return f.exp[int(f.log[x])+int(f.log[y])]
}

// An RSEncoder implements Reed-Solomon encoding
// over a given field using a given number of error correction bytes.
type RSEncoder struct {
	f    *Field
	c    int
	gen  []byte
	lgen []byte
	p    []byte
}

func (f *Field) gen(e int) (gen, lgen []byte) {
	// p = 1
	p := make([]byte, e+1)
	p[e] = 1

	for i := 0; i < e; i++ {
		// p *= (x + Exp(i))
		// p[j] = p[j]*Exp(i) + p[j+1].
		c := f.Exp(i)
		for j := 0; j < e; j++ {
			p[j] = f.Mul(p[j], c) ^ p[j+1]
		}
		p[e] = f.Mul(p[e], c)
	}

	// lp = log p.
	lp := make([]byte, e+1)
	for i, c := range p {
		if c == 0 {
			lp[i] = 255
		} else {
			lp[i] = byte(f.Log(c))
		}
	}

	return p, lp
}

// NewRSEncoder returns a new Reed-Solomon encoder
// over the given field and number of error correction bytes.
func NewRSEncoder(f *Field, c int) *RSEncoder {
	gen, lgen := f.gen(c)
	return &RSEncoder{f: f, c: c, gen: gen, lgen: lgen}
}

// ECC writes to check the error correcting code bytes
// for data using the given Reed-Solomon parameters.
func (rs *RSEncoder) ECC(data []byte, check []byte) {
	if len(check) < rs.c {
		panic("gf256: invalid check byte length")
	}
	if rs.c == 0 {
		return
	}

	// The check bytes are the remainder after dividing
	// data padded with c zeros by the generator polynomial.

	// p = data padded with c zeros.
	var p []byte
	n := len(data) + rs.c
	if len(rs.p) >= n {
		p = rs.p
	} else {
		p = make([]byte, n)
	}
	copy(p, data)
	for i := len(data); i < len(p); i++ {
		p[i] = 0
	}

	// Divide p by gen, leaving the remainder in p[len(data):].
	// p[0] is the most significant term in p, and
	// gen[0] is the most significant term in the generator,
	// which is always 1.
	// To avoid repeated work, we store various values as
	// lv, not v, where lv = log[v].
	f := rs.f
	lgen := rs.lgen[1:]
	for i := 0; i < len(data); i++ {
		c := p[i]
		if c == 0 {
			continue
		}
		q := p[i+1:]
		exp := f.exp[f.log[c]:]
		for j, lg := range lgen {
			if lg != 255 { // lgen uses 255 for log 0
				q[j] ^= exp[lg]
			}
		}
	}
	copy(check, p[len(data):])
	rs.p = p
}
