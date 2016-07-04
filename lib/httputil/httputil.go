// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package httputil

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"time"

	"github.com/syncthing/syncthing/lib/dialer"
)

// The Client performs HTTP operations.
type Client interface {
	Get(url string) (*http.Response, error)
	Post(url, ctype string, data io.Reader) (*http.Response, error)
	Do(req *http.Request) (*http.Response, error)
}

// No HTTP request may take longer than this to complete
const readTimeout = 30 * time.Minute

// The version that we claim in User-Agents. Can be changed by calling
// InitWithVersion().
var defaultVersion string

var (
	// The Default client is a normal HTTP/HTTPS client.
	Default Client

	// The Insecure client does not perform certificate validation.
	Insecure Client
)

func init() {
	// Run the init with a default version string so the package is usable
	// as is. Hopefully main.main() will run InitWithVersion() again to set
	// a real version for the User-Agent header.
	InitWithVersion("unknown-dev")
}

// InitWithVersion (re-)initializes the package with the given version
// string, which should be just the usual "v1.2.3" format. This sets the
// User-Agent for all requests performed by this package.
func InitWithVersion(version string) {
	defaultVersion = version
	Default = New(false)
	Insecure = New(true)
}

// New returns a new HTTP client, potentially with certificate checking disabled.
func New(insecure bool) Client {
	return newUserAgentClient(userAgent(defaultVersion), &http.Client{
		Timeout: readTimeout,
		Transport: &http.Transport{
			Dial:  dialer.Dial,
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecure,
			},
		},
	})
}

// WithClientCert returns a new HTTP client that can authenticate using the
// given client certificate, potentially with certificate checking disabled.
func WithClientCert(insecure bool, cert tls.Certificate) Client {
	return newUserAgentClient(userAgent(defaultVersion), &http.Client{
		Timeout: readTimeout,
		Transport: &http.Transport{
			Dial:  dialer.Dial,
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecure,
				Certificates:       []tls.Certificate{cert},
			},
		},
	})
}

// Get issues a GET to the specified URL using the Default client.
func Get(url string) (*http.Response, error) {
	return Default.Get(url)
}

func userAgent(version string) string {
	return fmt.Sprintf(`syncthing %s (%s %s-%s)`, version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

type userAgentClient struct {
	userAgent string
	next      Client
}

func newUserAgentClient(userAgent string, next Client) Client {
	return &userAgentClient{
		userAgent: userAgent,
		next:      next,
	}
}

func (a *userAgentClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return a.Do(req)
}

func (a *userAgentClient) Post(url, ctype string, data io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, data)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", ctype)
	return a.Do(req)
}

func (a *userAgentClient) Do(req *http.Request) (*http.Response, error) {
	// The GitHub asset server (S3?) requires application/octet-stream
	// specifically here. The */* should cover everything else, much as if
	// we didn't set the header at all.
	req.Header.Set("Accept", "application/octet-stream; */*")

	// Set our user agent as a courtesy to the server, and to possibly
	// tailor upgrade and other responses to the calling version.
	req.Header.Set("User-Agent", a.userAgent)

	return a.next.Do(req)
}
