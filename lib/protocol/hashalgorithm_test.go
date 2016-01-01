// Copyright (C) 2016 The Protocol Authors.

package protocol

import "testing"

/*
   0                   1                   2                   3
    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
   |                       Reserved                  | Hash  |D|P|R|
   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
*/
func TestHashAlgorithmFromFlagBits(t *testing.T) {
	// SHA256 is algorithm zero, shifted three bits to the left (for clarity,
	// I know it doesn't actually do anything).

	sha256 := uint32(0 << 3)

	h, err := HashAlgorithmFromFlagBits(sha256)
	if err != nil {
		t.Error(err)
	}
	if h != SHA256 {
		t.Error("Zero should have unmarshalled as SHA256")
	}

	// Any other algorithm is unknown
	unknown := uint32(1 << 3)

	_, err = HashAlgorithmFromFlagBits(unknown)
	if err == nil {
		t.Error("Unknown algo should not have unmarshalled")
	}
}
