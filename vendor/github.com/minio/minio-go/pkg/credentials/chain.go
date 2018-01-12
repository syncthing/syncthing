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

// A Chain will search for a provider which returns credentials
// and cache that provider until Retrieve is called again.
//
// The Chain provides a way of chaining multiple providers together
// which will pick the first available using priority order of the
// Providers in the list.
//
// If none of the Providers retrieve valid credentials Value, ChainProvider's
// Retrieve() will return the no credentials value.
//
// If a Provider is found which returns valid credentials Value ChainProvider
// will cache that Provider for all calls to IsExpired(), until Retrieve is
// called again after IsExpired() is true.
//
//     creds := credentials.NewChainCredentials(
//         []credentials.Provider{
//             &credentials.EnvAWSS3{},
//             &credentials.EnvMinio{},
//         })
//
//     // Usage of ChainCredentials.
//     mc, err := minio.NewWithCredentials(endpoint, creds, secure, "us-east-1")
//     if err != nil {
//          log.Fatalln(err)
//     }
//
type Chain struct {
	Providers []Provider
	curr      Provider
}

// NewChainCredentials returns a pointer to a new Credentials object
// wrapping a chain of providers.
func NewChainCredentials(providers []Provider) *Credentials {
	return New(&Chain{
		Providers: append([]Provider{}, providers...),
	})
}

// Retrieve returns the credentials value, returns no credentials(anonymous)
// if no credentials provider returned any value.
//
// If a provider is found with credentials, it will be cached and any calls
// to IsExpired() will return the expired state of the cached provider.
func (c *Chain) Retrieve() (Value, error) {
	for _, p := range c.Providers {
		creds, _ := p.Retrieve()
		// Always prioritize non-anonymous providers, if any.
		if creds.AccessKeyID == "" && creds.SecretAccessKey == "" {
			continue
		}
		c.curr = p
		return creds, nil
	}
	// At this point we have exhausted all the providers and
	// are left without any credentials return anonymous.
	return Value{
		SignerType: SignatureAnonymous,
	}, nil
}

// IsExpired will returned the expired state of the currently cached provider
// if there is one. If there is no current provider, true will be returned.
func (c *Chain) IsExpired() bool {
	if c.curr != nil {
		return c.curr.IsExpired()
	}

	return true
}
