// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

const (
	nonceSize    = 12
	aeadOverhead = 16
	aesKeySize   = 32
	aesBlockSize = 16
	segmentSize  = protocol.BlockSize - nonceSize - aeadOverhead
)

var (
	// <encryptedname>.sync-conflict-20060102-150405-7777777 <-> <name>.sync-conflict-20060102-150405-7777777.<ext>
	// <encryptedname>~20060102-150405 <-> <name>~20060102-150405.<ext>
	// ~syncthing~.<encryptedname>.tmp <-> ~syncthing~.<name>.<ext>.tmp
	// .syncthing.<encryptedname>.tmp <-> .syncthing.<name>.<ext>.tmp
	encryptedConflictRe     = regexp.MustCompile(`^(.+)(\.sync-conflict-\d{8}-\d{6}-[A-Z0-8]{7})$`)
	unencryptedConflictRe   = regexp.MustCompile(`^(.+)(\.sync-conflict-\d{8}-\d{6}-[A-Z0-8]{7})(\..+)?$`)
	encryptedVersioningRe   = regexp.MustCompile(`^(.+)(~\d{8}-\d{6})$`)
	unencryptedVersioningRe = regexp.MustCompile(`^(.+)(~\d{8}-\d{6})(\..+)?$`)
	tempNameRe              = regexp.MustCompile(`^([~\.]syncthing[~\.])(.*)(\.tmp)$`)
)

func newEncryptedFilesystem(rawUri string) (*encryptedFilesystem, error) {
	uri, err := url.Parse(rawUri)
	if err != nil {
		return nil, errors.New("invalid encrypted filesystem uri: " + err.Error())
	}

	var underlyingType FilesystemType
	underlyingType.UnmarshalText([]byte(uri.Scheme))

	unpaddedKey, err := base64.RawStdEncoding.DecodeString(uri.Host)
	key := pad(unpaddedKey, aesKeySize)
	if err != nil {
		return nil, errors.New("invalid key error: " + err.Error())
	}

	blockCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("block cipher error: " + err.Error())
	}

	aead, err := cipher.NewGCM(blockCipher)
	if err != nil {
		return nil, errors.New("aead cipher error: " + err.Error())
	}

	if len(uri.Path) < 1 {
		return nil, errors.New("no path specified")
	}

	path := uri.Path
	if (runtime.GOOS == "windows" && path[0] == '/') || (len(path) > 2 && path[0] == '/' && path[1] == '/') {
		path = path[1:]
	}

	underlyingFs := NewFilesystem(underlyingType, path)

	nonceManager, err := newNonceManager(underlyingFs, ".stfolder")
	if err != nil {
		return nil, errors.New("nonce manager: " + err.Error())
	}

	return &encryptedFilesystem{
		Filesystem: underlyingFs,
		uri:        rawUri,
		block:      blockCipher,
		aead:       aead,
		encNames:   (1<<1)&key[0] != 0,
		nonces:     nonceManager,
	}, nil
}

type encryptedFilesystem struct {
	Filesystem
	uri      string
	block    cipher.Block
	aead     cipher.AEAD
	encNames bool
	nonces   nonceManager
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
	fs.nonces.discardContentNonces(name)
	return fs.Filesystem.Remove(decryptedName)
}

func (fs *encryptedFilesystem) RemoveAll(name string) error {
	decryptedName, err := fs.decryptName(name)
	if err != nil {
		return err
	}
	fs.nonces.discardContentNonces(name)
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
	fs.nonces.discardContentNonces(decryptedOldName)
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
	// We only support a known set of patterns (or rather suffixes in this case)
	// These patterns will never have extensions, as encrypted names do not
	// have extensions
	for _, suffix := range []string{
		".sync-conflict-????????-??????-???????",
		"~[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]",
	} {
		if strings.HasSuffix(pattern, suffix) {
			// Remove the suffix, decrypt the name, reconstruct the pattern that
			// is readable by the plain text filesystem.
			decryptedName, err := fs.decryptName(strings.TrimSuffix(pattern, suffix))
			if err != nil {
				return nil, err
			}
			ext := filepath.Ext(decryptedName)
			decryptedName = strings.TrimSuffix(decryptedName, ext)
			names, err := fs.Filesystem.Glob(decryptedName + suffix + ext)
			if err != nil {
				return nil, err
			}
			return fs.encryptNames(names)
		}
	}
	panic("bug: unexpected pattern in encrypted filesystem")
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

func (fs *encryptedFilesystem) decryptPart(part string) (string, error) {
	part = strings.Replace(part, "-", "+", -1)
	part = strings.Replace(part, "_", "/", -1)
	partBytes, err := base64.RawStdEncoding.DecodeString(part)
	if err != nil {
		return "", err
	}

	// Nonce has to be aesBlockSize big.
	nonce := partBytes[:aesBlockSize]
	partBytes = partBytes[aesBlockSize:]

	decrypter := cipher.NewCFBDecrypter(fs.block, nonce)
	decrypter.XORKeyStream(partBytes, partBytes)

	partBytes, err = unpad(partBytes, aesBlockSize)
	if err != nil {
		return "", err
	}

	fs.nonces.setNameNonce(part, nonce)
	return string(partBytes), nil
}

func (fs encryptedFilesystem) encryptPart(part string) string {
	partBytes := []byte(part)
	partBytes = pad(partBytes, aesBlockSize)

	nonce := fs.nonces.getNameNonces(part)

	encrypter := cipher.NewCFBEncrypter(fs.block, nonce)
	encrypter.XORKeyStream(partBytes, partBytes)

	part = base64.RawStdEncoding.EncodeToString(append(nonce, partBytes...))
	part = strings.Replace(part, "+", "-", -1)
	part = strings.Replace(part, "/", "_", -1)
	return part
}

func (fs *encryptedFilesystem) decryptName(name string) (string, error) {
	// See encryption for comments.
	if !fs.encNames {
		return name, nil
	}

	return mapName(name, func(part string) (string, error) {
		if skipEncryption(part) {
			return part, nil
		}

		parts := tempNameRe.FindStringSubmatch(part)
		if len(parts) == 4 {
			decryptedName, err := fs.decryptName(parts[2])
			if err != nil {
				return "", err
			}
			parts[2] = decryptedName
			return strings.Join(parts[1:], ""), nil
		}

		parts = encryptedVersioningRe.FindStringSubmatch(part)
		if len(parts) == 3 {
			decryptedName, err := fs.decryptName(parts[1])
			if err != nil {
				return "", err
			}
			ext := filepath.Ext(decryptedName)
			parts[1] = strings.TrimSuffix(decryptedName, ext)
			parts = append(parts, ext)
			return strings.Join(parts[1:], ""), nil
		}

		parts = encryptedConflictRe.FindStringSubmatch(part)
		if len(parts) == 3 {
			decryptedName, err := fs.decryptName(parts[1])
			if err != nil {
				return "", err
			}
			ext := filepath.Ext(decryptedName)
			parts[1] = strings.TrimSuffix(decryptedName, ext)
			parts = append(parts, ext)
			return strings.Join(parts[1:], ""), nil
		}

		return fs.decryptPart(part)
	})

}

func (fs *encryptedFilesystem) encryptName(name string) (string, error) {
	if !fs.encNames {
		return name, nil
	}

	return mapName(name, func(part string) (string, error) {
		if skipEncryption(part) {
			return part, nil
		}

		// If it looks like a temporary name, encrypt only the name but not our
		// temp markers.
		parts := tempNameRe.FindStringSubmatch(part)
		if len(parts) == 4 {
			encryptedName, err := fs.encryptName(parts[2])
			if err != nil {
				return "", err
			}
			parts[2] = encryptedName
			return strings.Join(parts[1:], ""), nil
		}

		// If it looks like a versioned file, encrypt only the name and not version
		// information.
		parts = unencryptedVersioningRe.FindStringSubmatch(part)
		if len(parts) == 4 {
			encryptedName, err := fs.encryptName(parts[1] + parts[3])
			if err != nil {
				return "", err
			}
			parts[1] = encryptedName
			return strings.Join(parts[1:3], ""), nil
		}

		// If it looks like a conflict, encrypt only the base name and not version
		// information
		parts = unencryptedConflictRe.FindStringSubmatch(part)
		if len(parts) == 4 {
			encryptedName, err := fs.encryptName(parts[1] + parts[3])
			if err != nil {
				return "", err
			}
			parts[1] = encryptedName
			return strings.Join(parts[1:3], ""), nil
		}

		// Encrypt the whole thing.
		return fs.encryptPart(part), nil
	})
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
	// Interal files such as .stignore should not be encrypted.
	if skipEncryption(fd.Name()) {
		return fd, nil
	}

	file := &encryptedFile{
		fd:     fd,
		name:   name,
		fs:     fs,
		aead:   fs.aead,
		nonces: fs.nonces.getContentNonceStorage(name),
	}

	return file, nil
}

func (fs *encryptedFilesystem) encryptedFileInfo(name string, info FileInfo) (FileInfo, error) {
	return &encryptedFileInfo{
		FileInfo: info,
		name:     name,
		fs:       fs,
	}, nil
}

type encryptedFile struct {
	fd     File
	name   string
	offset int64
	mut    sync.Mutex
	fs     *encryptedFilesystem
	aead   cipher.AEAD
	nonces *nonceStorage
}

func (f *encryptedFile) Read(p []byte) (int, error) {
	f.mut.Lock()
	n, err := f.ReadAt(p, f.offset)
	f.offset += int64(n)
	f.mut.Unlock()
	return n, err
}

func (f *encryptedFile) Write(p []byte) (int, error) {
	f.mut.Lock()
	n, err := f.WriteAt(p, f.offset)
	f.offset += int64(n)
	f.mut.Unlock()
	return n, err
}

// Encryption stuff

// ReadAt stitches up multiple encrypted blocks of protocol.BlockSize and reads
// how many bytes client asks for. Technically we could assume all reads will
// be in block sizes, but if someone wraps us in bufio.Reader(), we might get
// strange sized reads, hence handle this better.
func (f *encryptedFile) ReadAt(p []byte, offset int64) (int, error) {
	// Get the block idx at the read offset
	startingBlock := int(offset / protocol.BlockSize)

	// Get intrablock position, of how much we need to discard.
	discard := int(offset % protocol.BlockSize)

	var readers []io.Reader

	// -1 because
	// x/protocol.BlockSize
	// where x < BlockSize = 0, so we end up doing one read
	// where x > BlockSize = 1+, so we end up doing two reads
	// where x = Blocksize = 1, so we end up doing two reads, yet we only need 1
	for i := 0; i <= (len(p)+discard-1)/protocol.BlockSize; i++ {
		block, err := f.readBlock(startingBlock + i)
		readers = append(readers, bytes.NewReader(block))
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return 0, err
			}
		}
	}

	// Construct a reader stitching the blocks
	reader := bufio.NewReader(io.MultiReader(readers...))

	// Discard however many bytes the user wants within the block
	if discard > 0 {
		_, err := reader.Discard(discard)
		if err != nil {
			return 0, err
		}
	}

	// MultiReader reads until first reader returning EOF (returns nil itself),
	// so we use a ReadFull to work around that, and handle it's unusual error.
	// Technically, we shouldn't need to do that and assume the code that
	// uses this handles non-full reads, but in reality, none of our own code
	// does a ReadFull, and assumes all writes return all data.
	n, err := io.ReadFull(reader, p)
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}

	return n, err
}

func (f *encryptedFile) readBlock(block int) ([]byte, error) {
	// Buffer for the block we are due to return
	buf := make([]byte, nonceSize+segmentSize+aeadOverhead)

	// Don't reference the nonce slice directly
	nonce := f.nonces.get(block)
	copy(buf, nonce)

	data := buf[nonceSize : nonceSize+segmentSize]

	// Read a segment from the file into right offset of buf.
	// This is going to be nonceSize+aeadOverhead less than protocol.BlockSize.
	n, err := f.fd.ReadAt(data, int64(block)*segmentSize)

	// Seal buffer, which does an in place modification and appends the signature (aeadOverhead)
	// onto the back of it.
	f.aead.Seal(data[:0], nonce, data[:n], nil)

	return buf[:nonceSize+n+aeadOverhead], err
}

func (f *encryptedFile) WriteAt(p []byte, offset int64) (int, error) {
	// All writes need to be block aligned.
	// Otherwise we'd have to support buffering which would be painful, as we
	// support WriteAt interface, hence handling two WriteAt's at overlapping
	// regions would be a mess, and might even require reading stuff back.
	if offset%protocol.BlockSize != 0 {
		panic("bug: unaligned write")
	}

	written := 0
	startingBlock := int(offset / protocol.BlockSize)
	for i := 0; i <= (len(p)-1)/protocol.BlockSize; i++ {
		// For blocks that are smaller than a full block, yet could still be
		// valid blocks.
		end := (i + 1) * protocol.BlockSize
		if end > len(p) {
			end = len(p)
		}
		data := p[i*protocol.BlockSize : end]
		if len(data) == 0 {
			break
		}
		n, err := f.writeBlock(startingBlock+i, data)
		written += n
		if err != nil {
			return written, err
		}
	}

	return written, nil
}

func (f *encryptedFile) writeBlock(block int, data []byte) (int, error) {
	if len(data) < nonceSize {
		return 0, io.ErrShortWrite
	}

	// Make a copy of the buffer, because we decrypt in-place
	buf := make([]byte, len(data)-nonceSize)
	copy(buf, data[nonceSize:])
	nonce := data[:nonceSize]

	buf, err := f.aead.Open(buf[:0], nonce, buf, nil)
	if err != nil {
		return 0, err
	}

	_, err = f.fd.WriteAt(buf, int64(block*segmentSize))
	if err != nil {
		return 0, err
	}

	f.nonces.set(block, nonce)

	return len(data), nil
}

// Standard stuff
func (f *encryptedFile) Name() string {
	return f.name
}

func (f *encryptedFile) Stat() (FileInfo, error) {
	stat, err := f.fd.Stat()
	if err != nil {
		return nil, err
	}
	return f.fs.encryptedFileInfo(f.name, stat)
}

func (f *encryptedFile) Truncate(size int64) error {
	// nonceStorage is not thread safe.
	f.nonces.reset(size)
	return f.fd.Truncate(size)
}

func (f *encryptedFile) Close() error {
	return f.fd.Close()
}

func (f *encryptedFile) Sync() error {
	return f.fd.Sync()
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
	sz := i.FileInfo.Size()

	blocks := sz / protocol.BlockSize
	if sz%protocol.BlockSize != 0 {
		blocks++
	}

	return sz + blocks*(aeadOverhead+nonceSize)
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

type pathMapper func(string) (string, error)

func mapName(path string, mapper pathMapper) (string, error) {
	parts := strings.Split(path, string(filepath.Separator))
	resultParts := make([]string, len(parts))
	for i, part := range parts {
		resultPart, err := mapper(part)
		if err != nil {
			return "", err
		}
		resultParts[i] = resultPart
	}
	// Don't use filepath.Join here as it removes things like ././ which we don't want
	return strings.Join(resultParts, string(filepath.Separator)), nil
}

func skipEncryption(name string) bool {
	return IsInternal(name) || name == "." || name == ".."
}
