package kcp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"

	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/tea"
)

var (
	initialVector = []byte{167, 115, 79, 156, 18, 172, 27, 1, 164, 21, 242, 193, 252, 120, 230, 107}
	saltxor       = `sH3CIVoF#rWLtJo6`
)

// BlockCrypt defines encryption/decryption methods for a given byte slice
type BlockCrypt interface {
	// Encrypt encrypts the whole block in src into dst.
	// Dst and src may point at the same memory.
	Encrypt(dst, src []byte)

	// Decrypt decrypts the whole block in src into dst.
	// Dst and src may point at the same memory.
	Decrypt(dst, src []byte)
}

// AESBlockCrypt implements BlockCrypt with AES
type AESBlockCrypt struct {
	encbuf []byte
	decbuf []byte
	block  cipher.Block
}

// NewAESBlockCrypt initates AES BlockCrypt by the given key
func NewAESBlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(AESBlockCrypt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	c.block = block
	c.encbuf = make([]byte, aes.BlockSize)
	c.decbuf = make([]byte, 2*aes.BlockSize)
	return c, nil
}

// Encrypt implements Encrypt interface
func (c *AESBlockCrypt) Encrypt(dst, src []byte) {
	encrypt(c.block, dst, src, c.encbuf)
}

// Decrypt implements Decrypt interface
func (c *AESBlockCrypt) Decrypt(dst, src []byte) {
	decrypt(c.block, dst, src, c.decbuf)
}

// TEABlockCrypt implements BlockCrypt with TEA
type TEABlockCrypt struct {
	encbuf []byte
	decbuf []byte
	block  cipher.Block
}

// NewTEABlockCrypt initate TEA BlockCrypt by the given key
func NewTEABlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(TEABlockCrypt)
	block, err := tea.NewCipherWithRounds(key, 16)
	if err != nil {
		return nil, err
	}
	c.block = block
	c.encbuf = make([]byte, tea.BlockSize)
	c.decbuf = make([]byte, 2*tea.BlockSize)
	return c, nil
}

// Encrypt implements Encrypt interface
func (c *TEABlockCrypt) Encrypt(dst, src []byte) {
	encrypt(c.block, dst, src, c.encbuf)
}

// Decrypt implements Decrypt interface
func (c *TEABlockCrypt) Decrypt(dst, src []byte) {
	decrypt(c.block, dst, src, c.decbuf)
}

// SimpleXORBlockCrypt implements BlockCrypt with simple xor to a table
type SimpleXORBlockCrypt struct {
	xortbl []byte
}

// NewSimpleXORBlockCrypt initate SimpleXORBlockCrypt by the given key
func NewSimpleXORBlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(SimpleXORBlockCrypt)
	c.xortbl = pbkdf2.Key(key, []byte(saltxor), 32, mtuLimit, sha1.New)
	return c, nil
}

// Encrypt implements Encrypt interface
func (c *SimpleXORBlockCrypt) Encrypt(dst, src []byte) {
	xorBytes(dst, src, c.xortbl)
}

// Decrypt implements Decrypt interface
func (c *SimpleXORBlockCrypt) Decrypt(dst, src []byte) {
	xorBytes(dst, src, c.xortbl)
}

// NoneBlockCrypt simple returns the plaintext
type NoneBlockCrypt struct {
	xortbl []byte
}

// NewNoneBlockCrypt initate NoneBlockCrypt by the given key
func NewNoneBlockCrypt(key []byte) (BlockCrypt, error) {
	return new(NoneBlockCrypt), nil
}

// Encrypt implements Encrypt interface
func (c *NoneBlockCrypt) Encrypt(dst, src []byte) {}

// Decrypt implements Decrypt interface
func (c *NoneBlockCrypt) Decrypt(dst, src []byte) {}

// packet encryption with local CFB mode
func encrypt(block cipher.Block, dst, src, buf []byte) {
	blocksize := block.BlockSize()
	tbl := buf[:blocksize]
	block.Encrypt(tbl, initialVector)
	n := len(src) / blocksize
	base := 0
	for i := 0; i < n; i++ {
		xorWords(dst[base:], src[base:], tbl)
		block.Encrypt(tbl, dst[base:])
		base += blocksize
	}
	xorBytes(dst[base:], src[base:], tbl)
}

func decrypt(block cipher.Block, dst, src, buf []byte) {
	blocksize := block.BlockSize()
	tbl := buf[:blocksize]
	next := buf[blocksize:]
	block.Encrypt(tbl, initialVector)
	n := len(src) / blocksize
	base := 0
	for i := 0; i < n; i++ {
		block.Encrypt(next, src[base:])
		xorWords(dst[base:], src[base:], tbl)
		tbl, next = next, tbl
		base += blocksize
	}
	xorBytes(dst[base:], src[base:], tbl)
}
