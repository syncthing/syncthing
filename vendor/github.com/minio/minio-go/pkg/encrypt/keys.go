/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage
 * Copyright 2017 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package encrypt

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
)

// Key - generic interface to encrypt/decrypt a key.
// We use it to encrypt/decrypt content key which is the key
// that encrypt/decrypt object data.
type Key interface {
	// Encrypt data using to the set encryption key
	Encrypt([]byte) ([]byte, error)
	// Decrypt data using to the set encryption key
	Decrypt([]byte) ([]byte, error)
}

// SymmetricKey - encrypts data with a symmetric master key
type SymmetricKey struct {
	masterKey []byte
}

// Encrypt passed bytes
func (s *SymmetricKey) Encrypt(plain []byte) ([]byte, error) {
	// Initialize an AES encryptor using a master key
	keyBlock, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return []byte{}, err
	}

	// Pad the key before encryption
	plain, _ = pkcs5Pad(plain, aes.BlockSize)

	encKey := []byte{}
	encPart := make([]byte, aes.BlockSize)

	// Encrypt the passed key by block
	for {
		if len(plain) < aes.BlockSize {
			break
		}
		// Encrypt the passed key
		keyBlock.Encrypt(encPart, plain[:aes.BlockSize])
		// Add the encrypted block to the total encrypted key
		encKey = append(encKey, encPart...)
		// Pass to the next plain block
		plain = plain[aes.BlockSize:]
	}
	return encKey, nil
}

// Decrypt passed bytes
func (s *SymmetricKey) Decrypt(cipher []byte) ([]byte, error) {
	// Initialize AES decrypter
	keyBlock, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, err
	}

	var plain []byte
	plainPart := make([]byte, aes.BlockSize)

	// Decrypt the encrypted data block by block
	for {
		if len(cipher) < aes.BlockSize {
			break
		}
		keyBlock.Decrypt(plainPart, cipher[:aes.BlockSize])
		// Add the decrypted block to the total result
		plain = append(plain, plainPart...)
		// Pass to the next cipher block
		cipher = cipher[aes.BlockSize:]
	}

	// Unpad the resulted plain data
	plain, err = pkcs5Unpad(plain, aes.BlockSize)
	if err != nil {
		return nil, err
	}

	return plain, nil
}

// NewSymmetricKey generates a new encrypt/decrypt crypto using
// an AES master key password
func NewSymmetricKey(b []byte) *SymmetricKey {
	return &SymmetricKey{masterKey: b}
}

// AsymmetricKey - struct which encrypts/decrypts data
// using RSA public/private certificates
type AsymmetricKey struct {
	publicKey  *rsa.PublicKey
	privateKey *rsa.PrivateKey
}

// Encrypt data using public key
func (a *AsymmetricKey) Encrypt(plain []byte) ([]byte, error) {
	cipher, err := rsa.EncryptPKCS1v15(rand.Reader, a.publicKey, plain)
	if err != nil {
		return nil, err
	}
	return cipher, nil
}

// Decrypt data using public key
func (a *AsymmetricKey) Decrypt(cipher []byte) ([]byte, error) {
	cipher, err := rsa.DecryptPKCS1v15(rand.Reader, a.privateKey, cipher)
	if err != nil {
		return nil, err
	}
	return cipher, nil
}

// NewAsymmetricKey - generates a crypto module able to encrypt/decrypt
// data using a pair for private and public key
func NewAsymmetricKey(privData []byte, pubData []byte) (*AsymmetricKey, error) {
	// Parse private key from passed data
	priv, err := x509.ParsePKCS8PrivateKey(privData)
	if err != nil {
		return nil, err
	}
	privKey, ok := priv.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("not a valid private key")
	}

	// Parse public key from passed data
	pub, err := x509.ParsePKIXPublicKey(pubData)
	if err != nil {
		return nil, err
	}

	pubKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not a valid public key")
	}

	// Associate the private key with the passed public key
	privKey.PublicKey = *pubKey

	return &AsymmetricKey{
		publicKey:  pubKey,
		privateKey: privKey,
	}, nil
}
