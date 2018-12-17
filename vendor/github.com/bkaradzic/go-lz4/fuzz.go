// +build gofuzz

package lz4

import "encoding/binary"

func Fuzz(data []byte) int {

	if len(data) < 4 {
		return 0
	}

	ln := binary.LittleEndian.Uint32(data)
	if ln > (1 << 21) {
		return 0
	}

	if _, err := Decode(nil, data); err != nil {
		return 0
	}

	return 1
}
