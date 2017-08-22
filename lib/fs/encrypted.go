// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net/url"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	nonceSize    = 12
	aeadOverhead = 16
	segmentSize  = protocol.BlockSize - nonceSize - aeadOverhead
)

var (
	errAlignment = errors.New("alignment error")
)

func newEncryptedFilesystem(rawUri string) Filesystem {
	uri, err := url.Parse(rawUri)
	if err != nil {
		return &errorFilesystem{
			fsType: FilesystemTypeEncrypted,
			uri:    rawUri,
			err:    errors.New("invalid encrypted filesystem uri:" + err.Error()),
		}
	}

	var underlyingType FilesystemType
	underlyingType.UnmarshalText([]byte(uri.Scheme))

	unpaddedKey, err := base64.StdEncoding.DecodeString(uri.Host)
	key := pad(unpaddedKey, 32)
	if err != nil {
		return &errorFilesystem{
			fsType: FilesystemTypeEncrypted,
			uri:    rawUri,
			err:    errors.New("invalid encryption key:" + err.Error()),
		}
	}

	cipher, err := aes.NewCipher(key)
	if err != nil {
		return &errorFilesystem{
			fsType: FilesystemTypeEncrypted,
			uri:    rawUri,
			err:    errors.New("invalid encryption key:" + err.Error()),
		}
	}

	underlyingFs := NewFilesystem(underlyingType, uri.RawPath)
	return &encryptedFilesystem{
		Filesystem: underlyingFs,
		uri:        rawUri,
		ciper:      cipher,
	}
}

type encryptedFilesystem struct {
	Filesystem
	uri   string
	ciper cipher.Block
}

func (fs *encryptedFilesystem) Chmod(name string, mode FileMode) error {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return err
	}
	return fs.Filesystem.Chmod(decryptedName, mode)
}
func (fs *encryptedFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return err
	}
	return fs.Filesystem.Chtimes(decryptedName, atime, mtime)
}

func (fs *encryptedFilesystem) Create(name string) (File, error) {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return nil, err
	}
	fd, err := fs.Filesystem.Create(decryptedName)
	if err != nil {
		return nil, err
	}
	return fs.encryptedFile(name, fd)
}

func (fs *encryptedFilesystem) CreateSymlink(name, target string) error {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return err
	}

	decryptedTarget, err := fs.decryptName(target)
	if err != nil {
		return err
	}

	return fs.Filesystem.CreateSymlink(decryptedName, decryptedTarget)
}

func (fs *encryptedFilesystem) DirNames(name string) ([]string, error) {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return nil, err
	}

	names, err := fs.Filesystem.DirNames(decryptedName)
	if err != nil {
		return nil, err
	}
	return fs.encryptNames(names)
}

func (fs *encryptedFilesystem) Lstat(name string) (FileInfo, error) {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return nil, err
	}

	stat, err := fs.Filesystem.Lstat(decryptedName)
	if err != nil {
		return nil, err
	}
	return fs.encryptedFileInfo(name, stat)
}

func (fs *encryptedFilesystem) Mkdir(name string, perm FileMode) error {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return err
	}

	return fs.Filesystem.Mkdir(decryptedName, perm)
}

func (fs *encryptedFilesystem) MkdirAll(name string, perm FileMode) error {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return err
	}

	return fs.Filesystem.MkdirAll(decryptedName, perm)
}

func (fs *encryptedFilesystem) Open(name string) (File, error) {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return nil, err
	}

	fd, err := fs.Filesystem.Open(decryptedName)
	if err != nil {
		return nil, err
	}

	return fs.encryptedFile(name, fd)
}

func (fs *encryptedFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return nil, err
	}

	fd, err := fs.Filesystem.OpenFile(decryptedName, flags, mode)
	if err != nil {
		return nil, err
	}

	return fs.encryptedFile(name, fd)
}

func (fs *encryptedFilesystem) ReadSymlink(name string) (string, error) {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return "", err
	}
	target, err := fs.Filesystem.ReadSymlink(decryptedName)
	if err != nil {
		return "", err
	}

	encryptedTarget, err := fs.encryptName(target)
	if err != nil {
		return "", err
	}
	return encryptedTarget, nil
}

func (fs *encryptedFilesystem) Remove(name string) error {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return err
	}

	return fs.Filesystem.Remove(decryptedName)
}

func (fs *encryptedFilesystem) RemoveAll(name string) error {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return err
	}

	return fs.Filesystem.RemoveAll(decryptedName)
}

func (fs *encryptedFilesystem) Rename(oldname, newname string) error {
	decryptedOldName, err := fs.decryptName(oldname)
	if err != nil {
		return err
	}
	decryptedNewName, err := fs.decryptName(newname)
	if err != nil {
		return err
	}
	return fs.Filesystem.Rename(decryptedOldName, decryptedNewName)
}

func (fs *encryptedFilesystem) Stat(name string) (FileInfo, error) {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return nil, err
	}

	stat, err := fs.Filesystem.Stat(decryptedName)
	if err != nil {
		return nil, err
	}

	return fs.encryptedFileInfo(name, stat)
}

func (fs *encryptedFilesystem) Unhide(name string) error {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return err
	}

	return fs.Filesystem.Unhide(decryptedName)
}

func (fs *encryptedFilesystem) Hide(name string) error {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return err
	}

	return fs.Filesystem.Hide(decryptedName)
}

func (fs *encryptedFilesystem) Glob(pattern string) ([]string, error) {
	// TODO
	decryptedPattern, err := fs.decryptName(pattern)
	if err != nil {
		return nil, err
	}

	names, err := fs.Filesystem.Glob(decryptedPattern)
	if err != nil {
		return nil, err
	}
	return fs.encryptNames(names)
}

func (fs *encryptedFilesystem) Roots() ([]string, error) {
	roots, err := fs.Roots()
	if err != nil {
		return nil, err
	}
	return fs.encryptNames(roots)
}

func (fs *encryptedFilesystem) Usage(name string) (Usage, error) {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return Usage{}, err
	}
	return fs.Filesystem.Usage(decryptedName)
}

func (fs *encryptedFilesystem) Type() FilesystemType {
	return FilesystemTypeEncrypted
}

func (fs *encryptedFilesystem) URI() string {
	return fs.uri
}

func (fs *encryptedFilesystem) decryptName(name string) (string, error) {
	// TODO
	return name, nil
}

func (fs *encryptedFilesystem) encryptName(name string) (string, error) {
	// TODO
	return name, nil
}

func (fs *encryptedFilesystem) encryptNames(names []string) ([]string, error) {
	encryptedNames := make([]string, len(names))
	for i := range names {
		encryptedName, err := fs.encryptName(names[i])
		if err != nil {
			return nil, err
		}
		encryptedNames[i] = encryptedName
	}
	return encryptedNames, nil
}

func (fs *encryptedFilesystem) encryptedFile(name string, fd File) (File, error) {
	return &encryptedFile{
		fd:   fd,
		name: name,
		fs:   fs,
	}, nil
}

func (fs *encryptedFilesystem) encryptedFileInfo(name string, info FileInfo) (FileInfo, error) {
	return &encryptedFileInfo{
		FileInfo: info,
		name:     name,
		fs:       fs,
	}, nil
}

type encryptedFileInfo struct {
	FileInfo
	name string
	fs   *encryptedFilesystem
}

func (i *encryptedFileInfo) Name() string {
	return i.name
}

func (i *encryptedFileInfo) Size() int64 {
	sz := i.Size()

	blocks := sz / protocol.BlockSize
	if sz%protocol.BlockSize != 0 {
		blocks++
	}

	return sz + blocks*(aeadOverhead+nonceSize)
}

/*
type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Writer
	io.WriterAt
	Name() string
	Truncate(size int64) error
	Stat() (FileInfo, error)
	Sync() error
}
*/

type nonceStorage [][]byte

func newNonceStorage(size int) nonceStorage {
	blocks := size / protocol.BlockSize
	if size%protocol.BlockSize != 0 {
		blocks++
	}
	return make(nonceStorage, blocks)
}

func (s nonceStorage) set(block int, nonce []byte) {
	s[block] = nonce
}

func (s nonceStorage) get(block int) []byte {
	nonce := s[block]
	if nonce == nil {
		nonce := make([]byte, nonceSize)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			panic(err.Error())
		}
		s.set(block, nonce)
	}
	return nonce
}

type encryptedFile struct {
	fd     File
	name   string
	fs     *encryptedFilesystem
	block  cipher.AEAD
	offset int64
	nonces [][]byte
}

// Encryption stuff

func (f *encryptedFile) Read(p []byte) (int, error) {
	n, err := f.ReadAt(p, f.offset)
	f.offset += int64(n)
	return n, err
}

func (f *encryptedFile) Write(p []byte) (int, error) {
	n, err := f.WriteAt(p, f.offset)
	f.offset += int64(n)
	return n, err
}

func (f *encryptedFile) ReadAt(p []byte, offset int64) (int, error) {
	return 0, nil
}

func (f *encryptedFile) WriteAt(p []byte, offset int64) (int, error) {
	return 0, nil
}

// Standard stuff
func (f *encryptedFile) Name() string {
	return f.name
}

func (f *encryptedFile) Close() error {
	return f.fd.Close()
}

func (f *encryptedFile) Sync() error {
	return f.fd.Sync()
}

func (f *encryptedFile) Stat() (FileInfo, error) {
	stat, err := f.fd.Stat()
	if err != nil {
		return nil, err
	}
	return f.fs.encryptedFileInfo(f.name, stat)
}

func (f *encryptedFile) Truncate(size int64) error {
	f.nonces = newNonceStorage(int(size))
	return f.fd.Truncate(size)
}

func pad(data []byte, size int) []byte {
	padding := size - len(data)%size
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

func unpad(buf []byte, size int) ([]byte, error) {
	bufLen := len(buf)
	if bufLen == 0 {
		return nil, errors.New("invalid padding size")
	}

	pad := buf[bufLen-1]
	padLen := int(pad)
	if padLen > bufLen || padLen > size {
		return nil, errors.New("invalid padding size")
	}

	for _, v := range buf[bufLen-padLen : bufLen-1] {
		if v != pad {
			return nil, errors.New("invalid padding")
		}
	}

	return buf[:bufLen-padLen], nil
}
