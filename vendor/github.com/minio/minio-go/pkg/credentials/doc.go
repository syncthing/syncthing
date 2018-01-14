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

// Package credentials provides credential retrieval and management
// for S3 compatible object storage.
//
// By default the Credentials.Get() will cache the successful result of a
// Provider's Retrieve() until Provider.IsExpired() returns true. At which
// point Credentials will call Provider's Retrieve() to get new credential Value.
//
// The Provider is responsible for determining when credentials have expired.
// It is also important to note that Credentials will always call Retrieve the
// first time Credentials.Get() is called.
//
// Example of using the environment variable credentials.
//
//     creds := NewFromEnv()
//     // Retrieve the credentials value
//     credValue, err := creds.Get()
//     if err != nil {
//         // handle error
//     }
//
// Example of forcing credentials to expire and be refreshed on the next Get().
// This may be helpful to proactively expire credentials and refresh them sooner
// than they would naturally expire on their own.
//
//     creds := NewFromIAM("")
//     creds.Expire()
//     credsValue, err := creds.Get()
//     // New credentials will be retrieved instead of from cache.
//
//
// Custom Provider
//
// Each Provider built into this package also provides a helper method to generate
// a Credentials pointer setup with the provider. To use a custom Provider just
// create a type which satisfies the Provider interface and pass it to the
// NewCredentials method.
//
//     type MyProvider struct{}
//     func (m *MyProvider) Retrieve() (Value, error) {...}
//     func (m *MyProvider) IsExpired() bool {...}
//
//     creds := NewCredentials(&MyProvider{})
//     credValue, err := creds.Get()
//
package credentials
