package missinggo_test

import (
	"fmt"

	"github.com/anacrolix/missinggo"
)

func ExampleNewRunLengthEncoder() {
	var s string
	rle := missinggo.NewRunLengthEncoder(func(e interface{}, count uint64) {
		s += fmt.Sprintf("%d%c", count, e)
	})
	for _, e := range "WWWWWWWWWWWWBWWWWWWWWWWWWBBBWWWWWWWWWWWWWWWWWWWWWWWWBWWWWWWWWWWWWWW" {
		rle.Append(e, 1)
	}
	rle.Flush()
	fmt.Println(s)
	// Output: 12W1B12W3B24W1B14W
}
