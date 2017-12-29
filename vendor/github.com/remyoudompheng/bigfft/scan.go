package bigfft

import (
	"math/big"
)

// FromDecimalString converts the base 10 string
// representation of a natural (non-negative) number
// into a *big.Int.
// Its asymptotic complexity is less than quadratic.
func FromDecimalString(s string) *big.Int {
	var sc scanner
	z := new(big.Int)
	sc.scan(z, s)
	return z
}

type scanner struct {
	// powers[i] is 10^(2^i * quadraticScanThreshold).
	powers []*big.Int
}

func (s *scanner) chunkSize(size int) (int, *big.Int) {
	if size <= quadraticScanThreshold {
		panic("size < quadraticScanThreshold")
	}
	pow := uint(0)
	for n := size; n > quadraticScanThreshold; n /= 2 {
		pow++
	}
	// threshold * 2^(pow-1) <= size < threshold * 2^pow
	return quadraticScanThreshold << (pow - 1), s.power(pow - 1)
}

func (s *scanner) power(k uint) *big.Int {
	for i := len(s.powers); i <= int(k); i++ {
		z := new(big.Int)
		if i == 0 {
			if quadraticScanThreshold%14 != 0 {
				panic("quadraticScanThreshold % 14 != 0")
			}
			z.Exp(big.NewInt(1e14), big.NewInt(quadraticScanThreshold/14), nil)
		} else {
			z.Mul(s.powers[i-1], s.powers[i-1])
		}
		s.powers = append(s.powers, z)
	}
	return s.powers[k]
}

func (s *scanner) scan(z *big.Int, str string) {
	if len(str) <= quadraticScanThreshold {
		z.SetString(str, 10)
		return
	}
	sz, pow := s.chunkSize(len(str))
	// Scan the left half.
	s.scan(z, str[:len(str)-sz])
	// FIXME: reuse temporaries.
	left := Mul(z, pow)
	// Scan the right half
	s.scan(z, str[len(str)-sz:])
	z.Add(z, left)
}

// quadraticScanThreshold is the number of digits
// below which big.Int.SetString is more efficient
// than subquadratic algorithms.
// 1232 digits fit in 4096 bits.
const quadraticScanThreshold = 1232
