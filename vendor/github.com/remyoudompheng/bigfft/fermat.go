package bigfft

import (
	"math/big"
)

// Arithmetic modulo 2^n+1.

// A fermat of length w+1 represents a number modulo 2^(w*_W) + 1. The last
// word is zero or one. A number has at most two representatives satisfying the
// 0-1 last word constraint.
type fermat nat

func (n fermat) String() string { return nat(n).String() }

func (z fermat) norm() {
	n := len(z) - 1
	c := z[n]
	if c == 0 {
		return
	}
	if z[0] >= c {
		z[n] = 0
		z[0] -= c
		return
	}
	// z[0] < z[n].
	subVW(z, z, c) // Substract c
	if c > 1 {
		z[n] -= c - 1
		c = 1
	}
	// Add back c.
	if z[n] == 1 {
		z[n] = 0
		return
	} else {
		addVW(z, z, 1)
	}
}

// Shift computes (x << k) mod (2^n+1).
func (z fermat) Shift(x fermat, k int) {
	if len(z) != len(x) {
		panic("len(z) != len(x) in Shift")
	}
	n := len(x) - 1
	// Shift by n*_W is taking the opposite.
	k %= 2 * n * _W
	if k < 0 {
		k += 2 * n * _W
	}
	neg := false
	if k >= n*_W {
		k -= n * _W
		neg = true
	}

	kw, kb := k/_W, k%_W

	z[n] = 1 // Add (-1)
	if !neg {
		for i := 0; i < kw; i++ {
			z[i] = 0
		}
		// Shift left by kw words.
		// x = a·2^(n-k) + b
		// x<<k = (b<<k) - a
		copy(z[kw:], x[:n-kw])
		b := subVV(z[:kw+1], z[:kw+1], x[n-kw:])
		if z[kw+1] > 0 {
			z[kw+1] -= b
		} else {
			subVW(z[kw+1:], z[kw+1:], b)
		}
	} else {
		for i := kw + 1; i < n; i++ {
			z[i] = 0
		}
		// Shift left and negate, by kw words.
		copy(z[:kw+1], x[n-kw:n+1])            // z_low = x_high
		b := subVV(z[kw:n], z[kw:n], x[:n-kw]) // z_high -= x_low
		z[n] -= b
	}
	// Add back 1.
	if z[n] > 0 {
		z[n]--
	} else if z[0] < ^big.Word(0) {
		z[0]++
	} else {
		addVW(z, z, 1)
	}
	// Shift left by kb bits
	shlVU(z, z, uint(kb))
	z.norm()
}

// ShiftHalf shifts x by k/2 bits the left. Shifting by 1/2 bit
// is multiplication by sqrt(2) mod 2^n+1 which is 2^(3n/4) - 2^(n/4).
// A temporary buffer must be provided in tmp.
func (z fermat) ShiftHalf(x fermat, k int, tmp fermat) {
	n := len(z) - 1
	if k%2 == 0 {
		z.Shift(x, k/2)
		return
	}
	u := (k - 1) / 2
	a := u + (3*_W/4)*n
	b := u + (_W/4)*n
	z.Shift(x, a)
	tmp.Shift(x, b)
	z.Sub(z, tmp)
}

// Add computes addition mod 2^n+1.
func (z fermat) Add(x, y fermat) fermat {
	if len(z) != len(x) {
		panic("Add: len(z) != len(x)")
	}
	addVV(z, x, y) // there cannot be a carry here.
	z.norm()
	return z
}

// Sub computes substraction mod 2^n+1.
func (z fermat) Sub(x, y fermat) fermat {
	if len(z) != len(x) {
		panic("Add: len(z) != len(x)")
	}
	n := len(y) - 1
	b := subVV(z[:n], x[:n], y[:n])
	b += y[n]
	// If b > 0, we need to subtract b<<n, which is the same as adding b.
	z[n] = x[n]
	if z[0] <= ^big.Word(0)-b {
		z[0] += b
	} else {
		addVW(z, z, b)
	}
	z.norm()
	return z
}

func (z fermat) Mul(x, y fermat) fermat {
	if len(x) != len(y) {
		panic("Mul: len(x) != len(y)")
	}
	n := len(x) - 1
	if n < 30 {
		z = z[:2*n+2]
		basicMul(z, x, y)
		z = z[:2*n+1]
	} else {
		var xi, yi, zi big.Int
		xi.SetBits(x)
		yi.SetBits(y)
		zi.SetBits(z)
		zb := zi.Mul(&xi, &yi).Bits()
		if len(zb) <= n {
			// Short product.
			copy(z, zb)
			for i := len(zb); i < len(z); i++ {
				z[i] = 0
			}
			return z
		}
		z = zb
	}
	// len(z) is at most 2n+1.
	if len(z) > 2*n+1 {
		panic("len(z) > 2n+1")
	}
	// We now have
	// z = z[:n] + 1<<(n*W) * z[n:2n+1]
	// which normalizes to:
	// z = z[:n] - z[n:2n] + z[2n]
	c1 := big.Word(0)
	if len(z) > 2*n {
		c1 = addVW(z[:n], z[:n], z[2*n])
	}
	c2 := big.Word(0)
	if len(z) >= 2*n {
		c2 = subVV(z[:n], z[:n], z[n:2*n])
	} else {
		m := len(z) - n
		c2 = subVV(z[:m], z[:m], z[n:])
		c2 = subVW(z[m:n], z[m:n], c2)
	}
	// Restore carries.
	// Substracting z[n] -= c2 is the same
	// as z[0] += c2
	z = z[:n+1]
	z[n] = c1
	c := addVW(z, z, c2)
	if c != 0 {
		panic("impossible")
	}
	z.norm()
	return z
}

// copied from math/big
//
// basicMul multiplies x and y and leaves the result in z.
// The (non-normalized) result is placed in z[0 : len(x) + len(y)].
func basicMul(z, x, y fermat) {
	// initialize z
	for i := 0; i < len(z); i++ {
		z[i] = 0
	}
	for i, d := range y {
		if d != 0 {
			z[len(x)+i] = addMulVVW(z[i:i+len(x)], x, d)
		}
	}
}
