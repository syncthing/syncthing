package kcp

import (
	"crypto/sha1"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

const cryptKey = "testkey"
const cryptSalt = "kcptest"

func TestAES(t *testing.T) {
	pass := pbkdf2.Key(key, []byte(salt), 4096, 32, sha1.New)
	bc, err := NewAESBlockCrypt(pass)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 4096)
	for i := 0; i < 4096; i++ {
		data[i] = byte(i & 0xff)
	}
	bc.Encrypt(data, data)
	bc.Decrypt(data, data)
	t.Log(data)
}

func TestTEA(t *testing.T) {
	pass := pbkdf2.Key(key, []byte(salt), 4096, 16, sha1.New)
	bc, err := NewTEABlockCrypt(pass)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 4096)
	for i := 0; i < 4096; i++ {
		data[i] = byte(i & 0xff)
	}
	bc.Encrypt(data, data)
	bc.Decrypt(data, data)
	t.Log(data)
}

func TestSimpleXOR(t *testing.T) {
	pass := pbkdf2.Key(key, []byte(salt), 4096, 16, sha1.New)
	bc, err := NewSimpleXORBlockCrypt(pass)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 4096)
	for i := 0; i < 4096; i++ {
		data[i] = byte(i & 0xff)
	}
	bc.Encrypt(data, data)
	bc.Decrypt(data, data)
	t.Log(data)
}
