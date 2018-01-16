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

package credentials

import "os"

// A EnvMinio retrieves credentials from the environment variables of the
// running process. EnvMinioironment credentials never expire.
//
// EnvMinioironment variables used:
//
// * Access Key ID:     MINIO_ACCESS_KEY.
// * Secret Access Key: MINIO_SECRET_KEY.
type EnvMinio struct {
	retrieved bool
}

// NewEnvMinio returns a pointer to a new Credentials object
// wrapping the environment variable provider.
func NewEnvMinio() *Credentials {
	return New(&EnvMinio{})
}

// Retrieve retrieves the keys from the environment.
func (e *EnvMinio) Retrieve() (Value, error) {
	e.retrieved = false

	id := os.Getenv("MINIO_ACCESS_KEY")
	secret := os.Getenv("MINIO_SECRET_KEY")

	signerType := SignatureV4
	if id == "" || secret == "" {
		signerType = SignatureAnonymous
	}

	e.retrieved = true
	return Value{
		AccessKeyID:     id,
		SecretAccessKey: secret,
		SignerType:      signerType,
	}, nil
}

// IsExpired returns if the credentials have been retrieved.
func (e *EnvMinio) IsExpired() bool {
	return !e.retrieved
}
