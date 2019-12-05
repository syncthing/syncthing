package protocol

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"time"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/crypto/nacl/secretbox"
)

func encryptFileInfo(fi FileInfo, key *[32]byte) FileInfo {
	// The entire FileInfo is encrypted with a random nonce, and stashed
	// together with that nonce.
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		panic("catastrophic randomness failure: " + err.Error())
	}
	bs, err := proto.Marshal(&fi)
	if err != nil {
		panic("impossible serialization mishap: " + err.Error())
	}
	out := secretbox.Seal(nonce[:], bs, &nonce, key)

	// The visible name of the file is set to the hash of the plaintext file
	// name and the key. This is stable and unguessable without knowing the key.
	hashedName := sha256.Sum256(append([]byte(fi.Name), (*key)[:]...))

	// The vector is set to something that is higher than any other version sent
	// previously, assuming people's clocks are correct. We do this because
	// there is no way for the insecure device on the other end to do proper
	// conflict resolution, so they will simply accept and keep whatever is the
	// latest version they see. The secure devices will decrypt the real
	// FileInfo, see the real Version, and act appropriately regardless of what
	// this fake version happens to be.
	version := Vector{
		Counters: []Counter{
			{
				ID:    1,
				Value: uint64(time.Now().UnixNano()),
			},
		},
	}

	// Construct the fake block list. Each block will be secretbox.Overhead
	// bytes larger than the corresponding real one, and have no hashes.
	var size int64
	var blocks []BlockInfo
	for i, b := range fi.Blocks {
		size += int64(b.Size)
		blocks = append(blocks, BlockInfo{
			Offset: size + int64(i*secretbox.Overhead),
			Size:   b.Size + secretbox.Overhead,
		})
	}

	// Construct the fake FileInfo. This is mostly just a wrapper around the
	// encrypted FileInfo and fake block list.
	//
	// The type is always set to "file". For things that are not files
	// (directories, symlinks) we set the Deleted bit so that they will have no
	// representation on the destination file system. All the needed info is in
	// the encrypted FileInfo. We also mark deleted things as deleted, so they
	// get cleaned out.
	enc := FileInfo{
		Name:        base32.HexEncoding.EncodeToString(hashedName[:]),
		Type:        FileInfoTypeFile,
		Size:        size,
		Permissions: 0644,
		ModifiedS:   1234567890, // Sat Feb 14 00:31:30 CET 2009
		Deleted:     fi.Deleted || fi.Type != FileInfoTypeFile,
		Version:     version,
		Blocks:      blocks,
		Encrypted:   out,
	}

	return enc
}
