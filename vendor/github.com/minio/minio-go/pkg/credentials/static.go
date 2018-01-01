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

// A Static is a set of credentials which are set programmatically,
// and will never expire.
type Static struct {
	Value
}

// NewStaticV2 returns a pointer to a new Credentials object
// wrapping a static credentials value provider, signature is
// set to v2. If access and secret are not specified then
// regardless of signature type set it Value will return
// as anonymous.
func NewStaticV2(id, secret, token string) *Credentials {
	return NewStatic(id, secret, token, SignatureV2)
}

// NewStaticV4 is similar to NewStaticV2 with similar considerations.
func NewStaticV4(id, secret, token string) *Credentials {
	return NewStatic(id, secret, token, SignatureV4)
}

// NewStatic returns a pointer to a new Credentials object
// wrapping a static credentials value provider.
func NewStatic(id, secret, token string, signerType SignatureType) *Credentials {
	return New(&Static{
		Value: Value{
			AccessKeyID:     id,
			SecretAccessKey: secret,
			SessionToken:    token,
			SignerType:      signerType,
		},
	})
}

// Retrieve returns the static credentials.
func (s *Static) Retrieve() (Value, error) {
	if s.AccessKeyID == "" || s.SecretAccessKey == "" {
		// Anonymous is not an error
		return Value{SignerType: SignatureAnonymous}, nil
	}
	return s.Value, nil
}

// IsExpired returns if the credentials are expired.
//
// For Static, the credentials never expired.
func (s *Static) IsExpired() bool {
	return false
}
