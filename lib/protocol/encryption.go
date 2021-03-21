// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"context"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/miscreant/miscreant.go"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sha256"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/scrypt"
)

const (
	nonceSize             = 24   // chacha20poly1305.NonceSizeX
	tagSize               = 16   // chacha20poly1305.Overhead()
	keySize               = 32   // fits both chacha20poly1305 and AES-SIV
	minPaddedSize         = 1024 // smallest block we'll allow
	blockOverhead         = tagSize + nonceSize
	maxPathComponent      = 200              // characters
	encryptedDirExtension = ".syncthing-enc" // for top level dirs
	miscreantAlgo         = "AES-SIV"
)

// The encryptedModel sits between the encrypted device and the model. It
// receives encrypted metadata and requests from the untrusted device, so it
// must decrypt those and answer requests by encrypting the data.
type encryptedModel struct {
	model      Model
	folderKeys map[string]*[keySize]byte // folder ID -> key
}

func (e encryptedModel) Index(deviceID DeviceID, folder string, files []FileInfo) error {
	if folderKey, ok := e.folderKeys[folder]; ok {
		// incoming index data to be decrypted
		if err := decryptFileInfos(files, folderKey); err != nil {
			return err
		}
	}
	return e.model.Index(deviceID, folder, files)
}

func (e encryptedModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) error {
	if folderKey, ok := e.folderKeys[folder]; ok {
		// incoming index data to be decrypted
		if err := decryptFileInfos(files, folderKey); err != nil {
			return err
		}
	}
	return e.model.IndexUpdate(deviceID, folder, files)
}

func (e encryptedModel) Request(deviceID DeviceID, folder, name string, blockNo, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (RequestResponse, error) {
	folderKey, ok := e.folderKeys[folder]
	if !ok {
		return e.model.Request(deviceID, folder, name, blockNo, size, offset, hash, weakHash, fromTemporary)
	}

	// Figure out the real file name, offset and size from the encrypted /
	// tweaked values.

	realName, err := decryptName(name, folderKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting name: %w", err)
	}
	realSize := size - blockOverhead
	realOffset := offset - int64(blockNo*blockOverhead)

	if size < minPaddedSize {
		return nil, errors.New("short request")
	}

	// Decrypt the block hash.

	fileKey := FileKey(realName, folderKey)
	var additional [8]byte
	binary.BigEndian.PutUint64(additional[:], uint64(realOffset))
	realHash, err := decryptDeterministic(hash, fileKey, additional[:])
	if err != nil {
		// "Legacy", no offset additional data?
		realHash, err = decryptDeterministic(hash, fileKey, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("decrypting block hash: %w", err)
	}

	// Perform that request and grab the data.

	resp, err := e.model.Request(deviceID, folder, realName, blockNo, realSize, realOffset, realHash, 0, false)
	if err != nil {
		return nil, err
	}

	// Encrypt the response. Blocks smaller than minPaddedSize are padded
	// with random data.

	data := resp.Data()
	if len(data) < minPaddedSize {
		nd := make([]byte, minPaddedSize)
		copy(nd, data)
		if _, err := rand.Read(nd[len(data):]); err != nil {
			panic("catastrophic randomness failure")
		}
		data = nd
	}
	enc := encryptBytes(data, fileKey)
	resp.Close()
	return rawResponse{enc}, nil
}

func (e encryptedModel) DownloadProgress(deviceID DeviceID, folder string, updates []FileDownloadProgressUpdate) error {
	if _, ok := e.folderKeys[folder]; !ok {
		return e.model.DownloadProgress(deviceID, folder, updates)
	}

	// Encrypted devices shouldn't send these - ignore them.
	return nil
}

func (e encryptedModel) ClusterConfig(deviceID DeviceID, config ClusterConfig) error {
	return e.model.ClusterConfig(deviceID, config)
}

func (e encryptedModel) Closed(conn Connection, err error) {
	e.model.Closed(conn, err)
}

// The encryptedConnection sits between the model and the encrypted device. It
// encrypts outgoing metadata and decrypts incoming responses.
type encryptedConnection struct {
	ConnectionInfo
	conn       Connection
	folderKeys map[string]*[keySize]byte // folder ID -> key
}

func (e encryptedConnection) Start() {
	e.conn.Start()
}

func (e encryptedConnection) ID() DeviceID {
	return e.conn.ID()
}

func (e encryptedConnection) Index(ctx context.Context, folder string, files []FileInfo) error {
	if folderKey, ok := e.folderKeys[folder]; ok {
		encryptFileInfos(files, folderKey)
	}
	return e.conn.Index(ctx, folder, files)
}

func (e encryptedConnection) IndexUpdate(ctx context.Context, folder string, files []FileInfo) error {
	if folderKey, ok := e.folderKeys[folder]; ok {
		encryptFileInfos(files, folderKey)
	}
	return e.conn.IndexUpdate(ctx, folder, files)
}

func (e encryptedConnection) Request(ctx context.Context, folder string, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
	folderKey, ok := e.folderKeys[folder]
	if !ok {
		return e.conn.Request(ctx, folder, name, blockNo, offset, size, hash, weakHash, fromTemporary)
	}

	// Encrypt / adjust the request parameters.

	origSize := size
	if size < minPaddedSize {
		// Make a request for minPaddedSize data instead of the smaller
		// block. We'll chop of the extra data later.
		size = minPaddedSize
	}
	encName := encryptName(name, folderKey)
	encOffset := offset + int64(blockNo*blockOverhead)
	encSize := size + blockOverhead

	// Perform that request, getting back and encrypted block.

	bs, err := e.conn.Request(ctx, folder, encName, blockNo, encOffset, encSize, nil, 0, false)
	if err != nil {
		return nil, err
	}

	// Return the decrypted block (or an error if it fails decryption)

	fileKey := FileKey(name, folderKey)
	bs, err = DecryptBytes(bs, fileKey)
	if err != nil {
		return nil, err
	}
	return bs[:origSize], nil
}

func (e encryptedConnection) DownloadProgress(ctx context.Context, folder string, updates []FileDownloadProgressUpdate) {
	if _, ok := e.folderKeys[folder]; !ok {
		e.conn.DownloadProgress(ctx, folder, updates)
	}

	// No need to send these
}

func (e encryptedConnection) ClusterConfig(config ClusterConfig) {
	e.conn.ClusterConfig(config)
}

func (e encryptedConnection) Close(err error) {
	e.conn.Close(err)
}

func (e encryptedConnection) Closed() bool {
	return e.conn.Closed()
}

func (e encryptedConnection) Statistics() Statistics {
	return e.conn.Statistics()
}

func encryptFileInfos(files []FileInfo, folderKey *[keySize]byte) {
	for i, fi := range files {
		files[i] = encryptFileInfo(fi, folderKey)
	}
}

// encryptFileInfo encrypts a FileInfo and wraps it into a new fake FileInfo
// with an encrypted name.
func encryptFileInfo(fi FileInfo, folderKey *[keySize]byte) FileInfo {
	fileKey := FileKey(fi.Name, folderKey)

	// The entire FileInfo is encrypted with a random nonce, and concatenated
	// with that nonce.

	bs, err := proto.Marshal(&fi)
	if err != nil {
		panic("impossible serialization mishap: " + err.Error())
	}
	encryptedFI := encryptBytes(bs, fileKey)

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
	// larger than the corresponding real one and have an encrypted hash.
	// Very small blocks will be padded upwards to minPaddedSize.
	//
	// The encrypted hash becomes just a "token" for the data -- it doesn't
	// help verifying it, but it lets the encrypted device do block level
	// diffs and data reuse properly when it gets a new version of a file.

	var offset int64
	blocks := make([]BlockInfo, len(fi.Blocks))
	for i, b := range fi.Blocks {
		if b.Size < minPaddedSize {
			b.Size = minPaddedSize
		}
		size := b.Size + blockOverhead

		// The offset goes into the encrypted block hash as additional data,
		// essentially mixing in with the nonce. This means a block hash
		// remains stable for the same data at the same offset, but doesn't
		// reveal the existence of identical data blocks at other offsets.
		var additional [8]byte
		binary.BigEndian.PutUint64(additional[:], uint64(b.Offset))
		hash := encryptDeterministic(b.Hash, fileKey, additional[:])

		blocks[i] = BlockInfo{
			Hash:   hash,
			Offset: offset,
			Size:   size,
		}
		offset += int64(size)
	}

	// Construct the fake FileInfo. This is mostly just a wrapper around the
	// encrypted FileInfo and fake block list. We'll represent symlinks as
	// directories, because they need some sort of on disk representation
	// but have no data outside of the metadata. Deletion and sequence
	// numbering are handled as usual.

	typ := FileInfoTypeFile
	if fi.Type != FileInfoTypeFile {
		typ = FileInfoTypeDirectory
	}
	enc := FileInfo{
		Name:         encryptName(fi.Name, folderKey),
		Type:         typ,
		Size:         offset, // new total file size
		Permissions:  0644,
		ModifiedS:    1234567890, // Sat Feb 14 00:31:30 CET 2009
		Deleted:      fi.Deleted,
		RawInvalid:   fi.IsInvalid(),
		Version:      version,
		Sequence:     fi.Sequence,
		RawBlockSize: fi.RawBlockSize + blockOverhead,
		Blocks:       blocks,
		Encrypted:    encryptedFI,
	}

	return enc
}

func decryptFileInfos(files []FileInfo, folderKey *[keySize]byte) error {
	for i, fi := range files {
		decFI, err := DecryptFileInfo(fi, folderKey)
		if err != nil {
			return err
		}
		files[i] = decFI
	}
	return nil
}

// DecryptFileInfo extracts the encrypted portion of a FileInfo, decrypts it
// and returns that.
func DecryptFileInfo(fi FileInfo, folderKey *[keySize]byte) (FileInfo, error) {
	realName, err := decryptName(fi.Name, folderKey)
	if err != nil {
		return FileInfo{}, err
	}

	fileKey := FileKey(realName, folderKey)
	dec, err := DecryptBytes(fi.Encrypted, fileKey)
	if err != nil {
		return FileInfo{}, err
	}

	var decFI FileInfo
	if err := proto.Unmarshal(dec, &decFI); err != nil {
		return FileInfo{}, err
	}
	return decFI, nil
}

var base32Hex = base32.HexEncoding.WithPadding(base32.NoPadding)

// encryptName encrypts the given string in a deterministic manner (the
// result is always the same for any given string) and encodes it in a
// filesystem-friendly manner.
func encryptName(name string, key *[keySize]byte) string {
	enc := encryptDeterministic([]byte(name), key, nil)
	return slashify(base32Hex.EncodeToString(enc))
}

// decryptName decrypts a string from encryptName
func decryptName(name string, key *[keySize]byte) (string, error) {
	name, err := deslashify(name)
	if err != nil {
		return "", err
	}
	bs, err := base32Hex.DecodeString(name)
	if err != nil {
		return "", err
	}
	dec, err := decryptDeterministic(bs, key, nil)
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

// encryptDeterministic encrypts bytes using AES-SIV
func encryptDeterministic(data []byte, key *[keySize]byte, additionalData []byte) []byte {
	aead, err := miscreant.NewAEAD(miscreantAlgo, key[:], 0)
	if err != nil {
		panic("cipher failure: " + err.Error())
	}
	return aead.Seal(nil, nil, data, additionalData)
}

// decryptDeterministic decrypts bytes using AES-SIV
func decryptDeterministic(data []byte, key *[keySize]byte, additionalData []byte) ([]byte, error) {
	aead, err := miscreant.NewAEAD(miscreantAlgo, key[:], 0)
	if err != nil {
		panic("cipher failure: " + err.Error())
	}
	return aead.Open(nil, nil, data, additionalData)
}

func encrypt(data []byte, nonce *[nonceSize]byte, key *[keySize]byte) []byte {
	aead, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		// Can only fail if the key is the wrong length
		panic("cipher failure: " + err.Error())
	}

	if aead.NonceSize() != nonceSize || aead.Overhead() != tagSize {
		// We want these values to be constant for our type declarations so
		// we don't use the values returned by the GCM, but we verify them
		// here.
		panic("crypto parameter mismatch")
	}

	// Data is appended to the nonce
	return aead.Seal(nonce[:], nonce[:], data, nil)
}

// DecryptBytes returns the decrypted bytes, or an error if decryption
// failed.
func DecryptBytes(data []byte, key *[keySize]byte) ([]byte, error) {
	if len(data) < blockOverhead {
		return nil, errors.New("data too short")
	}

	aead, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		// Can only fail if the key is the wrong length
		panic("cipher failure: " + err.Error())
	}

	if aead.NonceSize() != nonceSize || aead.Overhead() != tagSize {
		// We want these values to be constant for our type declarations so
		// we don't use the values returned by the GCM, but we verify them
		// here.
		panic("crypto parameter mismatch")
	}

	return aead.Open(nil, data[:nonceSize], data[nonceSize:], nil)
}

// randomNonce is a normal, cryptographically random nonce
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
		res[folder] = KeyFromPassword(folder, password)
	}
	return res
}

func knownBytes(folderID string) []byte {
	return []byte("syncthing" + folderID)
}

// KeyFromPassword uses key derivation to generate a stronger key from a
// probably weak password.
func KeyFromPassword(folderID, password string) *[keySize]byte {
	bs, err := scrypt.Key([]byte(password), knownBytes(folderID), 32768, 8, 1, keySize)
	if err != nil {
		panic("key derivation failure: " + err.Error())
	}
	if len(bs) != keySize {
		panic("key derivation failure: wrong number of bytes")
	}
	var key [keySize]byte
	copy(key[:], bs)
	return &key
}

var hkdfSalt = []byte("syncthing")

func FileKey(filename string, folderKey *[keySize]byte) *[keySize]byte {
	kdf := hkdf.New(sha256.New, append(folderKey[:], filename...), hkdfSalt, nil)
	var fileKey [keySize]byte
	n, err := io.ReadFull(kdf, fileKey[:])
	if err != nil || n != keySize {
		panic("hkdf failure")
	}
	return &fileKey
}

func PasswordToken(folderID, password string) []byte {
	return encryptDeterministic(knownBytes(folderID), KeyFromPassword(folderID, password), nil)
}

// slashify inserts slashes (and file extension) in the string to create an
// appropriate tree. ABCDEFGH... => A.syncthing-enc/BC/DEFGH... We can use
// forward slashes here because we're on the outside of native path formats,
// the slash is the wire format.
func slashify(s string) string {
	// We somewhat sloppily assume bytes == characters here, but the only
	// file names we should deal with are those that come from our base32
	// encoding.

	comps := make([]string, 0, len(s)/maxPathComponent+3)
	comps = append(comps, s[:1]+encryptedDirExtension)
	s = s[1:]
	comps = append(comps, s[:2])
	s = s[2:]

	for len(s) > maxPathComponent {
		comps = append(comps, s[:maxPathComponent])
		s = s[maxPathComponent:]
	}
	if len(s) > 0 {
		comps = append(comps, s)
	}
	return strings.Join(comps, "/")
}

// deslashify removes slashes and encrypted file extensions from the string.
// This is the inverse of slashify().
func deslashify(s string) (string, error) {
	if len(s) == 0 || !strings.HasPrefix(s[1:], encryptedDirExtension) {
		return "", fmt.Errorf("invalid encrypted path: %q", s)
	}
	s = s[:1] + s[1+len(encryptedDirExtension):]
	return strings.ReplaceAll(s, "/", ""), nil
}

type rawResponse struct {
	data []byte
}

func (r rawResponse) Data() []byte {
	return r.data
}

func (r rawResponse) Close() {}
func (r rawResponse) Wait()  {}

// IsEncryptedPath returns true if the path points at encrypted data. This is
// determined by checking for a sentinel string in the path.
func IsEncryptedPath(path string) bool {
	pathComponents := strings.Split(path, "/")
	if len(pathComponents) != 3 {
		return false
	}
	return isEncryptedParentFromComponents(pathComponents[:2])
}

// IsEncryptedParent returns true if the path points at a parent directory of
// encrypted data, i.e. is not a "real" directory. This is determined by
// checking for a sentinel string in the path.
func IsEncryptedParent(path string) bool {
	return isEncryptedParentFromComponents(strings.Split(path, "/"))
}

func isEncryptedParentFromComponents(pathComponents []string) bool {
	l := len(pathComponents)
	if l == 2 && len(pathComponents[1]) != 2 {
		return false
	} else if l == 0 {
		return false
	}
	if len(pathComponents[0]) == 0 {
		return false
	}
	if pathComponents[0][1:] != encryptedDirExtension {
		return false
	}
	if l < 2 {
		return true
	}
	for _, comp := range pathComponents[2:] {
		if len(comp) != maxPathComponent {
			return false
		}
	}
	return true
}
