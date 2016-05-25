// Package iter provides a syntantically different way to iterate over integers. That's it.
package iter

// N returns a slice of n 0-sized elements, suitable for ranging over.
//
// For example:
//
//    for i := range iter.N(10) {
//        fmt.Println(i)
//    }
//
// ... will print 0 to 9, inclusive.
//
// It does not cause any allocations.
func N(n int) []struct{} {
	return make([]struct{}, n)
}
