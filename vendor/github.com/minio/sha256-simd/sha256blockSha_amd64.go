//+build !noasm

package sha256

//go:noescape
func blockSha(h *[8]uint32, message []uint8)
