/*

Package rollinghash implements rolling versions of some hashes

*/
package rollinghash

import "hash"

type Roller interface {
	// Roll updates the hash of a rolling window from the entering byte.
	// A copy of the window is internally kept from the last Write().
	// This copy is updated along with the internal state of the checksum
	// in order to determine the new hash very quickly.
	Roll(b byte)
}

// rollinghash.Hash extends hash.Hash by adding the method Roll. A
// rollinghash.Hash can be updated byte by byte, by specifying which byte
// enters the window.
type Hash interface {
	hash.Hash
	Roller
}

// rollinghash.Hash32 extends hash.Hash by adding the method Roll. A
// rollinghash.Hash32 can be updated byte by byte, by specifying which
// byte enters the window.
type Hash32 interface {
	hash.Hash32
	Roller
}

// rollinghash.Hash64 extends hash.Hash by adding the method Roll. A
// rollinghash.Hash64 can be updated byte by byte, by specifying which
// byte enters the window.
type Hash64 interface {
	hash.Hash64
	Roller
}
