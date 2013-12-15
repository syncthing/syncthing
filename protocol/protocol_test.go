package protocol

import (
	"testing"
	"testing/quick"
)

func TestHeaderFunctions(t *testing.T) {
	f := func(ver, id, typ int) bool {
		ver = int(uint(ver) % 16)
		id = int(uint(id) % 4096)
		typ = int(uint(typ) % 256)
		h0 := header{ver, id, typ}
		h1 := decodeHeader(encodeHeader(h0))
		return h0 == h1
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestPad(t *testing.T) {
	tests := [][]int{
		{0, 0},
		{1, 3},
		{2, 2},
		{3, 1},
		{4, 0},
		{32, 0},
		{33, 3},
	}
	for _, tc := range tests {
		if p := pad(tc[0]); p != tc[1] {
			t.Errorf("Incorrect padding for %d bytes, %d != %d", tc[0], p, tc[1])
		}
	}
}
