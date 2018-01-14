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
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// Crypt mode - encryption or decryption
type cryptMode int

const (
	encryptMode cryptMode = iota
	decryptMode
)

// CBCSecureMaterials encrypts/decrypts data using AES CBC algorithm
type CBCSecureMaterials struct {

	// Data stream to encrypt/decrypt
	stream io.Reader

	// Last internal error
	err error

	// End of file reached
	eof bool

	// Holds initial data
	srcBuf *bytes.Buffer

	// Holds transformed data (encrypted or decrypted)
	dstBuf *bytes.Buffer

	// Encryption algorithm
	encryptionKey Key

	// Key to encrypts/decrypts data
	contentKey []byte

	// Encrypted form of contentKey
	cryptedKey []byte

	// Initialization vector
	iv []byte

	// matDesc - currently unused
	matDesc []byte

	// Indicate if we are going to encrypt or decrypt
	cryptMode cryptMode

	// Helper that encrypts/decrypts data
	blockMode cipher.BlockMode
}

// NewCBCSecureMaterials builds new CBC crypter module with
// the specified encryption key (symmetric or asymmetric)
func NewCBCSecureMaterials(key Key) (*CBCSecureMaterials, error) {
	if key == nil {
		return nil, errors.New("Unable to recognize empty encryption properties")
	}
	return &CBCSecureMaterials{
		srcBuf:        bytes.NewBuffer([]byte{}),
		dstBuf:        bytes.NewBuffer([]byte{}),
		encryptionKey: key,
		matDesc:       []byte("{}"),
	}, nil

}

// Close implements closes the internal stream.
func (s *CBCSecureMaterials) Close() error {
	closer, ok := s.stream.(io.Closer)
	if ok {
		return closer.Close()
	}
	return nil
}

// SetupEncryptMode - tells CBC that we are going to encrypt data
func (s *CBCSecureMaterials) SetupEncryptMode(stream io.Reader) error {
	// Set mode to encrypt
	s.cryptMode = encryptMode

	// Set underlying reader
	s.stream = stream

	s.eof = false
	s.srcBuf.Reset()
	s.dstBuf.Reset()

	var err error

	// Generate random content key
	s.contentKey = make([]byte, aes.BlockSize*2)
	if _, err := rand.Read(s.contentKey); err != nil {
		return err
	}
	// Encrypt content key
	s.cryptedKey, err = s.encryptionKey.Encrypt(s.contentKey)
	if err != nil {
		return err
	}
	// Generate random IV
	s.iv = make([]byte, aes.BlockSize)
	if _, err = rand.Read(s.iv); err != nil {
		return err
	}
	// New cipher
	encryptContentBlock, err := aes.NewCipher(s.contentKey)
	if err != nil {
		return err
	}

	s.blockMode = cipher.NewCBCEncrypter(encryptContentBlock, s.iv)

	return nil
}

// SetupDecryptMode - tells CBC that we are going to decrypt data
func (s *CBCSecureMaterials) SetupDecryptMode(stream io.Reader, iv string, key string) error {
	// Set mode to decrypt
	s.cryptMode = decryptMode

	// Set underlying reader
	s.stream = stream

	// Reset
	s.eof = false
	s.srcBuf.Reset()
	s.dstBuf.Reset()

	var err error

	// Get IV
	s.iv, err = base64.StdEncoding.DecodeString(iv)
	if err != nil {
		return err
	}

	// Get encrypted content key
	s.cryptedKey, err = base64.StdEncoding.DecodeString(key)
	if err != nil {
		return err
	}

	// Decrypt content key
	s.contentKey, err = s.encryptionKey.Decrypt(s.cryptedKey)
	if err != nil {
		return err
	}

	// New cipher
	decryptContentBlock, err := aes.NewCipher(s.contentKey)
	if err != nil {
		return err
	}

	s.blockMode = cipher.NewCBCDecrypter(decryptContentBlock, s.iv)
	return nil
}

// GetIV - return randomly generated IV (per S3 object), base64 encoded.
func (s *CBCSecureMaterials) GetIV() string {
	return base64.StdEncoding.EncodeToString(s.iv)
}

// GetKey - return content encrypting key (cek) in encrypted form, base64 encoded.
func (s *CBCSecureMaterials) GetKey() string {
	return base64.StdEncoding.EncodeToString(s.cryptedKey)
}

// GetDesc - user provided encryption material description in JSON (UTF8) format.
func (s *CBCSecureMaterials) GetDesc() string {
	return string(s.matDesc)
}

// Fill buf with encrypted/decrypted data
func (s *CBCSecureMaterials) Read(buf []byte) (n int, err error) {
	// Always fill buf from bufChunk at the end of this function
	defer func() {
		if s.err != nil {
			n, err = 0, s.err
		} else {
			n, err = s.dstBuf.Read(buf)
		}
	}()

	// Return
	if s.eof {
		return
	}

	// Fill dest buffer if its length is less than buf
	for !s.eof && s.dstBuf.Len() < len(buf) {

		srcPart := make([]byte, aes.BlockSize)
		dstPart := make([]byte, aes.BlockSize)

		// Fill src buffer
		for s.srcBuf.Len() < aes.BlockSize*2 {
			_, err = io.CopyN(s.srcBuf, s.stream, aes.BlockSize)
			if err != nil {
				break
			}
		}

		// Quit immediately for errors other than io.EOF
		if err != nil && err != io.EOF {
			s.err = err
			return
		}

		// Mark current encrypting/decrypting as finished
		s.eof = (err == io.EOF)

		if s.eof && s.cryptMode == encryptMode {
			if srcPart, err = pkcs5Pad(s.srcBuf.Bytes(), aes.BlockSize); err != nil {
				s.err = err
				return
			}
		} else {
			_, _ = s.srcBuf.Read(srcPart)
		}

		// Crypt srcPart content
		for len(srcPart) > 0 {

			// Crypt current part
			s.blockMode.CryptBlocks(dstPart, srcPart[:aes.BlockSize])

			// Unpad when this is the last part and we are decrypting
			if s.eof && s.cryptMode == decryptMode {
				dstPart, err = pkcs5Unpad(dstPart, aes.BlockSize)
				if err != nil {
					s.err = err
					return
				}
			}

			// Send crypted data to dstBuf
			if _, wErr := s.dstBuf.Write(dstPart); wErr != nil {
				s.err = wErr
				return
			}
			// Move to the next part
			srcPart = srcPart[aes.BlockSize:]
		}
	}
	return
}

// Unpad a set of bytes following PKCS5 algorithm
func pkcs5Unpad(buf []byte, blockSize int) ([]byte, error) {
	len := len(buf)
	if len == 0 {
		return nil, errors.New("buffer is empty")
	}
	pad := int(buf[len-1])
	if pad > len || pad > blockSize {
		return nil, errors.New("invalid padding size")
	}
	return buf[:len-pad], nil
}

// Pad a set of bytes following PKCS5 algorithm
func pkcs5Pad(buf []byte, blockSize int) ([]byte, error) {
	len := len(buf)
	pad := blockSize - (len % blockSize)
	padText := bytes.Repeat([]byte{byte(pad)}, pad)
	return append(buf, padText...), nil
}
