package farm

func rotate32(val uint32, shift uint) uint32 {
	return ((val >> shift) | (val << (32 - shift)))
}

func rotate64(val uint64, shift uint) uint64 {
	return ((val >> shift) | (val << (64 - shift)))
}

func fetch32(s []byte, idx int) uint32 {
	return uint32(s[idx+0]) | uint32(s[idx+1])<<8 | uint32(s[idx+2])<<16 | uint32(s[idx+3])<<24
}

func fetch64(s []byte, idx int) uint64 {
	return uint64(s[idx+0]) | uint64(s[idx+1])<<8 | uint64(s[idx+2])<<16 | uint64(s[idx+3])<<24 |
		uint64(s[idx+4])<<32 | uint64(s[idx+5])<<40 | uint64(s[idx+6])<<48 | uint64(s[idx+7])<<56
}
