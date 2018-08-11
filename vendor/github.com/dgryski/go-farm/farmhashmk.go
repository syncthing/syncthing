package farm

func hash32Len5to12(s []byte, seed uint32) uint32 {
	slen := len(s)
	a := uint32(len(s))
	b := uint32(len(s) * 5)
	c := uint32(9)
	d := b + seed
	a += fetch32(s, 0)
	b += fetch32(s, slen-4)
	c += fetch32(s, ((slen >> 1) & 4))
	return fmix(seed ^ mur(c, mur(b, mur(a, d))))
}

// Hash32 hashes a byte slice and returns a uint32 hash value
func Hash32(s []byte) uint32 {

	slen := len(s)

	if slen <= 24 {
		if slen <= 12 {
			if slen <= 4 {
				return hash32Len0to4(s, 0)
			}
			return hash32Len5to12(s, 0)
		}
		return hash32Len13to24Seed(s, 0)
	}

	// len > 24
	h := uint32(slen)
	g := c1 * uint32(slen)
	f := g
	a0 := rotate32(fetch32(s, slen-4)*c1, 17) * c2
	a1 := rotate32(fetch32(s, slen-8)*c1, 17) * c2
	a2 := rotate32(fetch32(s, slen-16)*c1, 17) * c2
	a3 := rotate32(fetch32(s, slen-12)*c1, 17) * c2
	a4 := rotate32(fetch32(s, slen-20)*c1, 17) * c2
	h ^= a0
	h = rotate32(h, 19)
	h = h*5 + 0xe6546b64
	h ^= a2
	h = rotate32(h, 19)
	h = h*5 + 0xe6546b64
	g ^= a1
	g = rotate32(g, 19)
	g = g*5 + 0xe6546b64
	g ^= a3
	g = rotate32(g, 19)
	g = g*5 + 0xe6546b64
	f += a4
	f = rotate32(f, 19) + 113
	iters := (slen - 1) / 20
	for {
		a := fetch32(s, 0)
		b := fetch32(s, 4)
		c := fetch32(s, 8)
		d := fetch32(s, 12)
		e := fetch32(s, 16)
		h += a
		g += b
		f += c
		h = mur(d, h) + e
		g = mur(c, g) + a
		f = mur(b+e*c1, f) + d
		f += g
		g += f
		s = s[20:]
		iters--
		if iters == 0 {
			break
		}
	}
	g = rotate32(g, 11) * c1
	g = rotate32(g, 17) * c1
	f = rotate32(f, 11) * c1
	f = rotate32(f, 17) * c1
	h = rotate32(h+g, 19)
	h = h*5 + 0xe6546b64
	h = rotate32(h, 17) * c1
	h = rotate32(h+f, 19)
	h = h*5 + 0xe6546b64
	h = rotate32(h, 17) * c1
	return h
}

// Hash32WithSeed hashes a byte slice and a uint32 seed and returns a uint32 hash value
func Hash32WithSeed(s []byte, seed uint32) uint32 {
	slen := len(s)

	if slen <= 24 {
		if slen >= 13 {
			return hash32Len13to24Seed(s, seed*c1)
		}
		if slen >= 5 {
			return hash32Len5to12(s, seed)
		}
		return hash32Len0to4(s, seed)
	}
	h := hash32Len13to24Seed(s[:24], seed^uint32(slen))
	return mur(Hash32(s[24:])+seed, h)
}
