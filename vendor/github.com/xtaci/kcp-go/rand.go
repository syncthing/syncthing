package kcp

import (
	"crypto/md5"
	"crypto/rand"
	"io"
)

// nonceMD5 is a nonce generator for each packet header
// which took the advantages of both MD5 and CSPRNG(like /dev/urandom).
// The benchmark shows it's faster than previous CSPRNG only method.
type nonceMD5 struct {
	data [md5.Size]byte
}

// Nonce fills a nonce into the provided slice with no more than md5.Size bytes
// the entropy will be updated whenever a leading 0 appears
func (n *nonceMD5) Fill(nonce []byte) {
	if n.data[0] == 0 { // 1/256 chance for entropy update
		io.ReadFull(rand.Reader, n.data[:])
	}
	n.data = md5.Sum(n.data[:])
	copy(nonce, n.data[:])
}
