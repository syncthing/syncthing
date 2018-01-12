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

// A EnvAWS retrieves credentials from the environment variables of the
// running process. EnvAWSironment credentials never expire.
//
// EnvAWSironment variables used:
//
// * Access Key ID:     AWS_ACCESS_KEY_ID or AWS_ACCESS_KEY.
// * Secret Access Key: AWS_SECRET_ACCESS_KEY or AWS_SECRET_KEY.
// * Secret Token:      AWS_SESSION_TOKEN.
type EnvAWS struct {
	retrieved bool
}

// NewEnvAWS returns a pointer to a new Credentials object
// wrapping the environment variable provider.
func NewEnvAWS() *Credentials {
	return New(&EnvAWS{})
}

// Retrieve retrieves the keys from the environment.
func (e *EnvAWS) Retrieve() (Value, error) {
	e.retrieved = false

	id := os.Getenv("AWS_ACCESS_KEY_ID")
	if id == "" {
		id = os.Getenv("AWS_ACCESS_KEY")
	}

	secret := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if secret == "" {
		secret = os.Getenv("AWS_SECRET_KEY")
	}

	signerType := SignatureV4
	if id == "" || secret == "" {
		signerType = SignatureAnonymous
	}

	e.retrieved = true
	return Value{
		AccessKeyID:     id,
		SecretAccessKey: secret,
		SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
		SignerType:      signerType,
	}, nil
}

// IsExpired returns if the credentials have been retrieved.
func (e *EnvAWS) IsExpired() bool {
	return !e.retrieved
}
