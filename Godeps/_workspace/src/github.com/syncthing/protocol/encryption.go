// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package protocol

import (
	"crypto/cipher"
	"crypto/aes"
	"crypto/rand"
	"io"
)

func Encrypt(in []byte, key[]byte) (out []byte, err error) {
	// Buffer needs to be multiples of aes.BlockSize
	var buf []byte
	if len(in)%aes.BlockSize != 0 {
		buf = make([]byte, ((len(in)/aes.BlockSize)+1)*aes.BlockSize)
		copy(buf, in)
	} else {
		buf = in
	}
	
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	// The IV needs to be unique, but not secure. Therefore it's common to
	// include it at the beginning of the ciphertext.
	out = make([]byte, aes.BlockSize+len(buf))
	iv := out[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err)
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(out[aes.BlockSize:], buf)

	return out, nil
}

func Decrypt(buf []byte, key []byte) (out []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	// The IV needs to be unique, but not secure. Therefore it's common to
	// include it at the beginning of the ciphertext.
	if len(buf) < aes.BlockSize {
		panic("ciphertext too short")
	}
	iv := buf[:aes.BlockSize]
	buf = buf[aes.BlockSize:]

	// CBC mode always works in whole blocks.
	if len(buf)%aes.BlockSize != 0 {
		panic("ciphertext is not a multiple of the block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)

	// CryptBlocks can work in-place if the two arguments are the same.
	mode.CryptBlocks(buf, buf)

	return buf, nil
}

// func Encrypt(buf []byte, label []byte, cert tls.Certificate) (out []byte, err error) {
// 	var ret []byte

// 	// Certificate stuff
// 	pub, err := x509.ParseCertificate(cert.Certificate[0])
// 	if err != nil {
// 		l.Debugln("error:", err)
// 		return nil, err
// 	}

// 	pubkey := pub.PublicKey.(*rsa.PublicKey)

// 	// now to encrypting
// 	// each encrypted chunk may only be ((pubkey.N.BitLen() + 7) / 8) - 11 byte big, so we may have to cut here

// 	sha := sha256.New()

// 	k := ((pubkey.N.BitLen() + 7) / 8) -2*sha.Size()-2

// 	var offset int
	
// 	for i := 0; i < len(buf); i += k {
// 		if i + k > len(buf) {
// 			k = len(buf) - i
// 		}
// 		out, err := rsa.EncryptOAEP(sha, rand.Reader, pubkey, buf[i:i+k], label) // Outputs 384 Bytes
// 		if err != nil {
// 			l.Debugln("Error Encrypting:", err)
// 			return nil, err
// 		}

// 		ret = append(ret, out...)

// 		offset += len(out)
// 	}

// 	return ret, nil
// }

// func Decrypt(buf []byte, label []byte, key *rsa.PrivateKey) (ret []byte, err error) {
// 	// now to encrypting
// 	// each encrypted chunk may only be ((pubkey.N.BitLen() + 7) / 8) - 11 byte big, so we may have to cut her

// 	k := 384

// 	out := make([]byte, 384)
// 	sha := sha256.New()
	
// 	for i := 0; i < len(buf); i += k {
// 		if i + k > len(buf) {
// 			k = len(buf) - i
// 		}
// 		out, err = rsa.DecryptOAEP(sha, rand.Reader, key, buf[i:i+k], label) // Outputs 384 Bytes
// 		if err != nil {
// 			l.Debugln("Error decrypting:", err)
// 			return nil, err
// 		}

// 		ret = append(ret, out...)
// 	}

// 	return ret, nil
// }