// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"
)

const (
	nonceSize              = 12 // cipher.gcmStandardNonceSize
	tagSize                = 16 // cipher.gcmTagSize
	keySize                = 32 // AES-256
	blockOverhead          = tagSize + nonceSize
	maxNameComponent       = 80 // characters
	EncryptedFileExtension = ".syncthing-enc"
)

// nonceSalt is a random salt we use for PBKDF2 when generating
// deterministic nonces.
var nonceSalt = [...]byte{
	0x83, 0xf6, 0x92, 0x44, 0x5f, 0x33, 0x00, 0xdb,
	0x14, 0x37, 0xe9, 0x69, 0x59, 0x09, 0x87, 0x3c,
	0x2b, 0x10, 0x03, 0x4d, 0x48, 0xfb, 0x34, 0x8f,
	0x0b, 0x0d, 0xfd, 0xe9, 0x7e, 0xe2, 0xc9, 0xef,
}

// keySalt is a random salt we use for PBKDF2 when generating
// encryption keys.
var keySalt = [...]byte{
	0x8e, 0x13, 0x3c, 0x96, 0x26, 0xfd, 0x87, 0xcc,
	0x03, 0x29, 0xa7, 0x84, 0xfa, 0x4e, 0xd9, 0xe5,
	0x5d, 0x3b, 0x2f, 0xa3, 0xa9, 0x72, 0x0f, 0x6b,
	0x5e, 0x91, 0xbb, 0xad, 0xe2, 0x49, 0xd7, 0x9d,
}

// The encryptedModel sits between the encrypted device and the model. It
// receives encrypted metadata and requests, so it must decrypt those and answer
// requests by encrypting the data.
type encryptedModel struct {
	Model
	keys map[string]*[keySize]byte // folder ID -> key
}

func (e encryptedModel) Index(deviceID DeviceID, folder string, files []FileInfo) error {
	if key, ok := e.keys[folder]; ok {
		if err := decryptFileInfos(files, key); err != nil {
			return err
		}
	}
	return e.Model.Index(deviceID, folder, files)
}

func (e encryptedModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) error {
	if key, ok := e.keys[folder]; ok {
		if err := decryptFileInfos(files, key); err != nil {
			return err
		}
	}
	return e.Model.IndexUpdate(deviceID, folder, files)
}

func (e encryptedModel) Request(deviceID DeviceID, folder, name string, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (RequestResponse, error) {
	key, ok := e.keys[folder]
	if !ok {
		return e.Model.Request(deviceID, folder, name, size, offset, hash, weakHash, fromTemporary)
	}

	// Figure out the real file name, offset and size from the encrypted /
	// tweaked values.

	realName, err := decryptName(name, key)
	if err != nil {
		return nil, err
	}
	realSize := size - blockOverhead
	realOffset := offset - int64(weakHash*blockOverhead)

	// Perform that request and grab the data.

	resp, err := e.Model.Request(deviceID, folder, realName, realSize, realOffset, nil, 0, false)
	if err != nil {
		if resp != nil {
			resp.Close()
		}
		return nil, err
	}

	// Encrypt the response.

	data := resp.Data()
	enc := encryptBytes(data, key)
	resp.Close()
	return rawResponse{enc}, nil
}

func (e encryptedModel) DownloadProgress(deviceID DeviceID, folder string, updates []FileDownloadProgressUpdate) error {
	if _, ok := e.keys[folder]; !ok {
		return e.Model.DownloadProgress(deviceID, folder, updates)
	}

	// The updates contain nonsense names and sizes, so we ignore them.
	return nil
}

// The encryptedConnection sits between the model and the encrypted device. It
// encrypts outgoing metadata and decrypts incoming responses.
type encryptedConnection struct {
	Connection
	keys map[string]*[keySize]byte // folder ID -> key
}

func (e encryptedConnection) Index(ctx context.Context, folder string, files []FileInfo) error {
	if key, ok := e.keys[folder]; ok {
		encryptFileInfos(files, key)
	}
	return e.Connection.Index(ctx, folder, files)
}

func (e encryptedConnection) IndexUpdate(ctx context.Context, folder string, files []FileInfo) error {
	if key, ok := e.keys[folder]; ok {
		encryptFileInfos(files, key)
	}
	return e.Connection.IndexUpdate(ctx, folder, files)
}

func (e encryptedConnection) Request(ctx context.Context, folder string, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
	key, ok := e.keys[folder]
	if !ok {
		return e.Connection.Request(ctx, folder, name, blockNo, offset, size, hash, weakHash, fromTemporary)
	}

	name = encryptName(name, key)
	offset += int64(blockNo * blockOverhead)
	size += blockOverhead

	bs, err := e.Connection.Request(ctx, folder, name, blockNo, offset, size, nil, uint32(blockNo), false)
	if err != nil {
		return nil, err
	}

	return decryptBytes(bs, key)
}

func (e encryptedConnection) DownloadProgress(ctx context.Context, folder string, updates []FileDownloadProgressUpdate) {
	if _, ok := e.keys[folder]; !ok {
		e.Connection.DownloadProgress(ctx, folder, updates)
	}

	// No need to send these
}

func encryptFileInfos(files []FileInfo, key *[keySize]byte) {
	for i, fi := range files {
		files[i] = encryptFileInfo(fi, key)
	}
}

// encryptFileInfo encrypts a FileInfo and wraps it into a new fake FileInfo
// with an encrypted name.
func encryptFileInfo(fi FileInfo, key *[keySize]byte) FileInfo {
	// The entire FileInfo is encrypted with a random nonce, and concatenated
	// with that nonce.

	bs, err := proto.Marshal(&fi)
	if err != nil {
		panic("impossible serialization mishap: " + err.Error())
	}
	encryptedFI := encryptBytes(bs, key)

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

	// Construct the fake block list. Each block will be blockOverhead bytes
	// larger than the corresponding real one, have an encrypted hash and
	// the block number in the weak hash.
	//
	// The encrypted hash becomes just a "token" for the data -- it doesn't
	// help verifying it, but it lets the encrypted device to block level
	// diffs and data reuse properly when it gets a new version of a file.
	//
	// Stuffing the block number in the weak hash is an ugly hack that
	// avoids a couple of other protocol changes, as it is a value that is
	// propagated through to the block request. It helps the other end
	// figure out the actual block offset to look at, given that the offset
	// we get from the encrypted side is tainted by an unknown number of
	// blockOverheads.

	var offset int64
	blocks := make([]BlockInfo, len(fi.Blocks))
	for i, b := range fi.Blocks {
		size := b.Size + blockOverhead
		blocks[i] = BlockInfo{
			Offset:   offset,
			Size:     size,
			Hash:     encryptDeterministic(b.Hash, key),
			WeakHash: uint32(i),
		}
		offset += int64(size)
	}

	// Construct the fake FileInfo. This is mostly just a wrapper around the
	// encrypted FileInfo and fake block list.

	typ := FileInfoTypeFile
	if fi.Type != FileInfoTypeFile {
		typ = FileInfoTypeDirectory
	}
	enc := FileInfo{
		Name:         encryptName(fi.Name, key),
		Type:         typ,
		Size:         offset,
		Permissions:  0644,
		ModifiedS:    1234567890, // Sat Feb 14 00:31:30 CET 2009
		Deleted:      fi.Deleted,
		Version:      version,
		Sequence:     fi.Sequence,
		RawBlockSize: fi.RawBlockSize + blockOverhead,
		Blocks:       blocks,
		Encrypted:    encryptedFI,
	}

	return enc
}

func decryptFileInfos(files []FileInfo, key *[keySize]byte) error {
	for i, fi := range files {
		decFI, err := decryptFileInfo(fi, key)
		if err != nil {
			return err
		}
		files[i] = decFI
	}
	return nil
}

// decryptFileInfo extracts the encrypted portion of a FileInfo, decrypts it
// and returns that.
func decryptFileInfo(fi FileInfo, key *[keySize]byte) (FileInfo, error) {
	dec, err := decryptBytes(fi.Encrypted, key)
	if err != nil {
		return FileInfo{}, err
	}

	var decFI FileInfo
	if err := proto.Unmarshal(dec, &decFI); err != nil {
		return FileInfo{}, err
	}
	return decFI, nil
}

// encryptName encrypts the given string in a deterministic manner (the
// result is always the same for any given string) and encodes it in a
// filesystem-friendly manner.
func encryptName(name string, key *[keySize]byte) string {
	enc := encryptDeterministic([]byte(name), key)
	b32enc := base32.HexEncoding.EncodeToString(enc)
	return slashify(b32enc)
}

// decryptName decrypts a string from encryptName
func decryptName(name string, key *[keySize]byte) (string, error) {
	name = deslashify(name)
	bs, err := base32.HexEncoding.DecodeString(name)
	if err != nil {
		return "", err
	}
	dec, err := decryptBytes(bs, key)
	if err != nil {
		return "", err
	}

	return string(dec), nil
}

// encryptBytes encrypts bytes with a random nonce
func encryptBytes(data []byte, key *[keySize]byte) []byte {
	nonce := randomNonce()
	return encrypt(data, nonce, key)
}

// encryptDeterministic encrypts bytes with a nonce based on the data and
// key.
func encryptDeterministic(data []byte, key *[keySize]byte) []byte {
	nonce := deterministicNonce(data, key)
	return encrypt(data, nonce, key)
}

func encrypt(data []byte, nonce *[nonceSize]byte, key *[keySize]byte) []byte {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		// Can only fail if the key is the wrong length
		panic("cipher failure: " + err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		// Can only fail if the crypto isn't able to do GCM
		panic("cipher failure: " + err.Error())
	}

	if gcm.NonceSize() != nonceSize || gcm.Overhead() != tagSize {
		// We want these values to be constant so we don't use the returns
		// from the GCM, but we verify them.
		panic("crypto parameter mismatch")
	}

	return gcm.Seal(nonce[:], nonce[:], data, nil)
}

// decryptBytes returns the decrypted bytes, or an error if decryption
// failed.
func decryptBytes(data []byte, key *[keySize]byte) ([]byte, error) {
	if len(data) < blockOverhead {
		return nil, errors.New("data too short")
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		// Can only fail if the key is the wrong length
		panic("cipher failure: " + err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		// Can only fail if the crypto isn't able to do GCM
		panic("cipher failure: " + err.Error())
	}

	if gcm.NonceSize() != nonceSize || gcm.Overhead() != tagSize {
		// We want these values to be constant so we don't use the returns
		// from the GCM, but we verify them.
		panic("crypto parameter mismatch")
	}

	return gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
}

// deterministicNonce is a nonce based on the hash of data and key using
// PBKDF2
func deterministicNonce(data []byte, key *[keySize]byte) *[nonceSize]byte {
	bs := pbkdf2.Key(append(data, (*key)[:]...), nonceSalt[:], 1024, nonceSize, sha256.New)
	if len(bs) != nonceSize {
		panic("pkdf2 failure")
	}
	var nonce [nonceSize]byte
	copy(nonce[:], bs)
	return &nonce
}

// randomNonce is a randomly generated nonce
func randomNonce() *[nonceSize]byte {
	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		panic("catastrophic randomness failure: " + err.Error())
	}
	return &nonce
}

// keysFromPasswords converts a set of folder ID to password into a set of
// folder ID to encryption key, using our key derivation function.
func keysFromPasswords(passwords map[string]string) map[string]*[keySize]byte {
	res := make(map[string]*[keySize]byte, len(passwords))
	for folder, password := range passwords {
		res[folder] = keyFromPassword(password)
	}
	return res
}

// keyFromPassword uses key derivation to generate a strong key from a probably weak
// password.
func keyFromPassword(password string) *[keySize]byte {
	bs, err := scrypt.Key([]byte(password), keySalt[:], 32768, 8, 1, 32)
	if err != nil {
		panic("key derivation failure: " + err.Error())
	}
	if len(bs) != 32 {
		panic("key derivation failure: wrong number of bytes")
	}
	var key [keySize]byte
	copy(key[:], bs)
	return &key
}

func slashify(s string) string {
	if len(s) <= maxNameComponent {
		return s + EncryptedFileExtension
	}

	comps := make([]string, 0, len(s)/maxNameComponent+1)
	for {
		if len(s) < maxNameComponent {
			comps = append(comps, s+EncryptedFileExtension)
			break
		}

		comps = append(comps, s[:maxNameComponent]+EncryptedFileExtension)
		s = s[maxNameComponent:]
	}
	return strings.Join(comps, "/")
}

func deslashify(s string) string {
	s = strings.ReplaceAll(s, EncryptedFileExtension, "")
	return strings.ReplaceAll(s, "/", "")
}

type rawResponse struct {
	data []byte
}

func (r rawResponse) Data() []byte {
	return r.data
}

func (r rawResponse) Close() {}
func (r rawResponse) Wait()  {}
