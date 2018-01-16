// Package bigfft implements multiplication of big.Int using FFT.
//
// The implementation is based on the Schönhage-Strassen method
// using integer FFT modulo 2^n+1.
package bigfft

import (
	"math/big"
	"unsafe"
)

const _W = int(unsafe.Sizeof(big.Word(0)) * 8)

type nat []big.Word

func (n nat) String() string {
	v := new(big.Int)
	v.SetBits(n)
	return v.String()
}

// fftThreshold is the size (in words) above which FFT is used over
// Karatsuba from math/big.
//
// TestCalibrate seems to indicate a threshold of 60kbits on 32-bit
// arches and 110kbits on 64-bit arches.
var fftThreshold = 1800

// Mul computes the product x*y and returns z.
// It can be used instead of the Mul method of
// *big.Int from math/big package.
func Mul(x, y *big.Int) *big.Int {
	xwords := len(x.Bits())
	ywords := len(y.Bits())
	if xwords > fftThreshold && ywords > fftThreshold {
		return mulFFT(x, y)
	}
	return new(big.Int).Mul(x, y)
}

func mulFFT(x, y *big.Int) *big.Int {
	var xb, yb nat = x.Bits(), y.Bits()
	zb := fftmul(xb, yb)
	z := new(big.Int)
	z.SetBits(zb)
	if x.Sign()*y.Sign() < 0 {
		z.Neg(z)
	}
	return z
}

// A FFT size of K=1<<k is adequate when K is about 2*sqrt(N) where
// N = x.Bitlen() + y.Bitlen().

func fftmul(x, y nat) nat {
	k, m := fftSize(x, y)
	xp := polyFromNat(x, k, m)
	yp := polyFromNat(y, k, m)
	rp := xp.Mul(&yp)
	return rp.Int()
}

// fftSizeThreshold[i] is the maximal size (in bits) where we should use
// fft size i.
var fftSizeThreshold = [...]int64{0, 0, 0,
	4 << 10, 8 << 10, 16 << 10, // 5 
	32 << 10, 64 << 10, 1 << 18, 1 << 20, 3 << 20, // 10
	8 << 20, 30 << 20, 100 << 20, 300 << 20, 600 << 20,
}

// returns the FFT length k, m the number of words per chunk
// such that m << k is larger than the number of words
// in x*y.
func fftSize(x, y nat) (k uint, m int) {
	words := len(x) + len(y)
	bits := int64(words) * int64(_W)
	k = uint(len(fftSizeThreshold))
	for i := range fftSizeThreshold {
		if fftSizeThreshold[i] > bits {
			k = uint(i)
			break
		}
	}
	// The 1<<k chunks of m words must have N bits so that
	// 2^N-1 is larger than x*y. That is, m<<k > words
	m = words>>k + 1
	return
}

// valueSize returns the length (in words) to use for polynomial
// coefficients, to compute a correct product of polynomials P*Q
// where deg(P*Q) < K (== 1<<k) and where coefficients of P and Q are
// less than b^m (== 1 << (m*_W)).
// The chosen length (in bits) must be a multiple of 1 << (k-extra).
func valueSize(k uint, m int, extra uint) int {
	// The coefficients of P*Q are less than b^(2m)*K
	// so we need W * valueSize >= 2*m*W+K
	n := 2*m*_W + int(k) // necessary bits
	K := 1 << (k - extra)
	if K < _W {
		K = _W
	}
	n = ((n / K) + 1) * K // round to a multiple of K
	return n / _W
}

// poly represents an integer via a polynomial in Z[x]/(x^K+1)
// where K is the FFT length and b^m is the computation basis 1<<(m*_W).
// If P = a[0] + a[1] x + ... a[n] x^(K-1), the associated natural number
// is P(b^m).
type poly struct {
	k uint  // k is such that K = 1<<k.
	m int   // the m such that P(b^m) is the original number.
	a []nat // a slice of at most K m-word coefficients.
}

// polyFromNat slices the number x into a polynomial
// with 1<<k coefficients made of m words.
func polyFromNat(x nat, k uint, m int) poly {
	p := poly{k: k, m: m}
	length := len(x)/m + 1
	p.a = make([]nat, length)
	for i := range p.a {
		if len(x) < m {
			p.a[i] = make(nat, m)
			copy(p.a[i], x)
			break
		}
		p.a[i] = x[:m]
		x = x[m:]
	}
	return p
}

// Int evaluates back a poly to its integer value.
func (p *poly) Int() nat {
	length := len(p.a)*p.m + 1
	if na := len(p.a); na > 0 {
		length += len(p.a[na-1])
	}
	n := make(nat, length)
	m := p.m
	np := n
	for i := range p.a {
		l := len(p.a[i])
		c := addVV(np[:l], np[:l], p.a[i])
		if np[l] < ^big.Word(0) {
			np[l] += c
		} else {
			addVW(np[l:], np[l:], c)
		}
		np = np[m:]
	}
	n = trim(n)
	return n
}

func trim(n nat) nat {
	for i := range n {
		if n[len(n)-1-i] != 0 {
			return n[:len(n)-i]
		}
	}
	return nil
}

// Mul multiplies p and q modulo X^K-1, where K = 1<<p.k.
// The product is done via a Fourier transform.
func (p *poly) Mul(q *poly) poly {
	// extra=2 because:
	// * some power of 2 is a K-th root of unity when n is a multiple of K/2.
	// * 2 itself is a square (see fermat.ShiftHalf)
	n := valueSize(p.k, p.m, 2)

	pv, qv := p.Transform(n), q.Transform(n)
	rv := pv.Mul(&qv)
	r := rv.InvTransform()
	r.m = p.m
	return r
}

// A polValues represents the value of a poly at the powers of a
// K-th root of unity θ=2^(l/2) in Z/(b^n+1)Z, where b^n = 2^(K/4*l).
type polValues struct {
	k      uint     // k is such that K = 1<<k.
	n      int      // the length of coefficients, n*_W a multiple of K/4.
	values []fermat // a slice of K (n+1)-word values
}

// Transform evaluates p at θ^i for i = 0...K-1, where
// θ is a K-th primitive root of unity in Z/(b^n+1)Z.
func (p *poly) Transform(n int) polValues {
	k := p.k
	inputbits := make([]big.Word, (n+1)<<k)
	input := make([]fermat, 1<<k)
	// Now computed q(ω^i) for i = 0 ... K-1
	valbits := make([]big.Word, (n+1)<<k)
	values := make([]fermat, 1<<k)
	for i := range values {
		input[i] = inputbits[i*(n+1) : (i+1)*(n+1)]
		if i < len(p.a) {
			copy(input[i], p.a[i])
		}
		values[i] = fermat(valbits[i*(n+1) : (i+1)*(n+1)])
	}
	fourier(values, input, false, n, k)
	return polValues{k, n, values}
}

// InvTransform reconstructs p (modulo X^K - 1) from its
// values at θ^i for i = 0..K-1.
func (v *polValues) InvTransform() poly {
	k, n := v.k, v.n

	// Perform an inverse Fourier transform to recover p.
	pbits := make([]big.Word, (n+1)<<k)
	p := make([]fermat, 1<<k)
	for i := range p {
		p[i] = fermat(pbits[i*(n+1) : (i+1)*(n+1)])
	}
	fourier(p, v.values, true, n, k)
	// Divide by K, and untwist q to recover p.
	u := make(fermat, n+1)
	a := make([]nat, 1<<k)
	for i := range p {
		u.Shift(p[i], -int(k))
		copy(p[i], u)
		a[i] = nat(p[i])
	}
	return poly{k: k, m: 0, a: a}
}

// NTransform evaluates p at θω^i for i = 0...K-1, where
// θ is a (2K)-th primitive root of unity in Z/(b^n+1)Z
// and ω = θ².
func (p *poly) NTransform(n int) polValues {
	k := p.k
	if len(p.a) >= 1<<k {
		panic("Transform: len(p.a) >= 1<<k")
	}
	// θ is represented as a shift.
	θshift := (n * _W) >> k
	// p(x) = a_0 + a_1 x + ... + a_{K-1} x^(K-1)
	// p(θx) = q(x) where
	// q(x) = a_0 + θa_1 x + ... + θ^(K-1) a_{K-1} x^(K-1)
	//
	// Twist p by θ to obtain q.
	tbits := make([]big.Word, (n+1)<<k)
	twisted := make([]fermat, 1<<k)
	src := make(fermat, n+1)
	for i := range twisted {
		twisted[i] = fermat(tbits[i*(n+1) : (i+1)*(n+1)])
		if i < len(p.a) {
			for i := range src {
				src[i] = 0
			}
			copy(src, p.a[i])
			twisted[i].Shift(src, θshift*i)
		}
	}

	// Now computed q(ω^i) for i = 0 ... K-1
	valbits := make([]big.Word, (n+1)<<k)
	values := make([]fermat, 1<<k)
	for i := range values {
		values[i] = fermat(valbits[i*(n+1) : (i+1)*(n+1)])
	}
	fourier(values, twisted, false, n, k)
	return polValues{k, n, values}
}

// InvTransform reconstructs a polynomial from its values at
// roots of x^K+1. The m field of the returned polynomial
// is unspecified.
func (v *polValues) InvNTransform() poly {
	k := v.k
	n := v.n
	θshift := (n * _W) >> k

	// Perform an inverse Fourier transform to recover q.
	qbits := make([]big.Word, (n+1)<<k)
	q := make([]fermat, 1<<k)
	for i := range q {
		q[i] = fermat(qbits[i*(n+1) : (i+1)*(n+1)])
	}
	fourier(q, v.values, true, n, k)

	// Divide by K, and untwist q to recover p.
	u := make(fermat, n+1)
	a := make([]nat, 1<<k)
	for i := range q {
		u.Shift(q[i], -int(k)-i*θshift)
		copy(q[i], u)
		a[i] = nat(q[i])
	}
	return poly{k: k, m: 0, a: a}
}

// fourier performs an unnormalized Fourier transform
// of src, a length 1<<k vector of numbers modulo b^n+1
// where b = 1<<_W.
func fourier(dst []fermat, src []fermat, backward bool, n int, k uint) {
	var rec func(dst, src []fermat, size uint)
	tmp := make(fermat, n+1)  // pre-allocate temporary variables.
	tmp2 := make(fermat, n+1) // pre-allocate temporary variables.

	// The recursion function of the FFT.
	// The root of unity used in the transform is ω=1<<(ω2shift/2).
	// The source array may use shifted indices (i.e. the i-th
	// element is src[i << idxShift]).
	rec = func(dst, src []fermat, size uint) {
		idxShift := k - size
		ω2shift := (4 * n * _W) >> size
		if backward {
			ω2shift = -ω2shift
		}

		// Easy cases.
		if len(src[0]) != n+1 || len(dst[0]) != n+1 {
			panic("len(src[0]) != n+1 || len(dst[0]) != n+1")
		}
		switch size {
		case 0:
			copy(dst[0], src[0])
			return
		case 1:
			dst[0].Add(src[0], src[1<<idxShift]) // dst[0] = src[0] + src[1]
			dst[1].Sub(src[0], src[1<<idxShift]) // dst[1] = src[0] - src[1]
			return
		}

		// Let P(x) = src[0] + src[1<<idxShift] * x + ... + src[K-1 << idxShift] * x^(K-1)
		// The P(x) = Q1(x²) + x*Q2(x²)
		// where Q1's coefficients are src with indices shifted by 1
		// where Q2's coefficients are src[1<<idxShift:] with indices shifted by 1

		// Split destination vectors in halves.
		dst1 := dst[:1<<(size-1)]
		dst2 := dst[1<<(size-1):]
		// Transform Q1 and Q2 in the halves.
		rec(dst1, src, size-1)
		rec(dst2, src[1<<idxShift:], size-1)

		// Reconstruct P's transform from transforms of Q1 and Q2.
		// dst[i]            is dst1[i] + ω^i * dst2[i]
		// dst[i + 1<<(k-1)] is dst1[i] + ω^(i+K/2) * dst2[i]
		//
		for i := range dst1 {
			tmp.ShiftHalf(dst2[i], i*ω2shift, tmp2) // ω^i * dst2[i]
			dst2[i].Sub(dst1[i], tmp)
			dst1[i].Add(dst1[i], tmp)
		}
	}
	rec(dst, src, k)
}

// Mul returns the pointwise product of p and q.
func (p *polValues) Mul(q *polValues) (r polValues) {
	n := p.n
	r.k, r.n = p.k, p.n
	r.values = make([]fermat, len(p.values))
	bits := make([]big.Word, len(p.values)*(n+1))
	buf := make(fermat, 8*n)
	for i := range r.values {
		r.values[i] = bits[i*(n+1) : (i+1)*(n+1)]
		z := buf.Mul(p.values[i], q.values[i])
		copy(r.values[i], z)
	}
	return
}
