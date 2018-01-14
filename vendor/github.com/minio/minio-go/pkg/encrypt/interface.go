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

// Package encrypt implements a generic interface to encrypt any stream of data.
// currently this package implements two types of encryption
// - Symmetric encryption using AES.
// - Asymmetric encrytion using RSA.
package encrypt

import "io"

// Materials - provides generic interface to encrypt any stream of data.
type Materials interface {

	// Closes the wrapped stream properly, initiated by the caller.
	Close() error

	// Returns encrypted/decrypted data, io.Reader compatible.
	Read(b []byte) (int, error)

	// Get randomly generated IV, base64 encoded.
	GetIV() (iv string)

	// Get content encrypting key (cek) in encrypted form, base64 encoded.
	GetKey() (key string)

	// Get user provided encryption material description in
	// JSON (UTF8) format. This is not used, kept for future.
	GetDesc() (desc string)

	// Setup encrypt mode, further calls of Read() function
	// will return the encrypted form of data streamed
	// by the passed reader
	SetupEncryptMode(stream io.Reader) error

	// Setup decrypted mode, further calls of Read() function
	// will return the decrypted form of data streamed
	// by the passed reader
	SetupDecryptMode(stream io.Reader, iv string, key string) error
}
