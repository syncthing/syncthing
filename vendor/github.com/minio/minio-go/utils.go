/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage
 * Copyright 2015-2017 Minio, Inc.
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

package minio

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/minio/minio-go/pkg/s3utils"
)

// xmlDecoder provide decoded value in xml.
func xmlDecoder(body io.Reader, v interface{}) error {
	d := xml.NewDecoder(body)
	return d.Decode(v)
}

// sum256 calculate sha256sum for an input byte array, returns hex encoded.
func sum256Hex(data []byte) string {
	hash := sha256.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}

// sumMD5Base64 calculate md5sum for an input byte array, returns base64 encoded.
func sumMD5Base64(data []byte) string {
	hash := md5.New()
	hash.Write(data)
	return base64.StdEncoding.EncodeToString(hash.Sum(nil))
}

// getEndpointURL - construct a new endpoint.
func getEndpointURL(endpoint string, secure bool) (*url.URL, error) {
	if strings.Contains(endpoint, ":") {
		host, _, err := net.SplitHostPort(endpoint)
		if err != nil {
			return nil, err
		}
		if !s3utils.IsValidIP(host) && !s3utils.IsValidDomain(host) {
			msg := "Endpoint: " + endpoint + " does not follow ip address or domain name standards."
			return nil, ErrInvalidArgument(msg)
		}
	} else {
		if !s3utils.IsValidIP(endpoint) && !s3utils.IsValidDomain(endpoint) {
			msg := "Endpoint: " + endpoint + " does not follow ip address or domain name standards."
			return nil, ErrInvalidArgument(msg)
		}
	}
	// If secure is false, use 'http' scheme.
	scheme := "https"
	if !secure {
		scheme = "http"
	}

	// Construct a secured endpoint URL.
	endpointURLStr := scheme + "://" + endpoint
	endpointURL, err := url.Parse(endpointURLStr)
	if err != nil {
		return nil, err
	}

	// Validate incoming endpoint URL.
	if err := isValidEndpointURL(*endpointURL); err != nil {
		return nil, err
	}
	return endpointURL, nil
}

// closeResponse close non nil response with any response Body.
// convenient wrapper to drain any remaining data on response body.
//
// Subsequently this allows golang http RoundTripper
// to re-use the same connection for future requests.
func closeResponse(resp *http.Response) {
	// Callers should close resp.Body when done reading from it.
	// If resp.Body is not closed, the Client's underlying RoundTripper
	// (typically Transport) may not be able to re-use a persistent TCP
	// connection to the server for a subsequent "keep-alive" request.
	if resp != nil && resp.Body != nil {
		// Drain any remaining Body and then close the connection.
		// Without this closing connection would disallow re-using
		// the same connection for future uses.
		//  - http://stackoverflow.com/a/17961593/4465767
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}
}

var (
	// Hex encoded string of nil sha256sum bytes.
	emptySHA256Hex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// Sentinel URL is the default url value which is invalid.
	sentinelURL = url.URL{}
)

// Verify if input endpoint URL is valid.
func isValidEndpointURL(endpointURL url.URL) error {
	if endpointURL == sentinelURL {
		return ErrInvalidArgument("Endpoint url cannot be empty.")
	}
	if endpointURL.Path != "/" && endpointURL.Path != "" {
		return ErrInvalidArgument("Endpoint url cannot have fully qualified paths.")
	}
	if strings.Contains(endpointURL.Host, ".s3.amazonaws.com") {
		if !s3utils.IsAmazonEndpoint(endpointURL) {
			return ErrInvalidArgument("Amazon S3 endpoint should be 's3.amazonaws.com'.")
		}
	}
	if strings.Contains(endpointURL.Host, ".googleapis.com") {
		if !s3utils.IsGoogleEndpoint(endpointURL) {
			return ErrInvalidArgument("Google Cloud Storage endpoint should be 'storage.googleapis.com'.")
		}
	}
	return nil
}

// Verify if input expires value is valid.
func isValidExpiry(expires time.Duration) error {
	expireSeconds := int64(expires / time.Second)
	if expireSeconds < 1 {
		return ErrInvalidArgument("Expires cannot be lesser than 1 second.")
	}
	if expireSeconds > 604800 {
		return ErrInvalidArgument("Expires cannot be greater than 7 days.")
	}
	return nil
}

// make a copy of http.Header
func cloneHeader(h http.Header) http.Header {
	h2 := make(http.Header, len(h))
	for k, vv := range h {
		vv2 := make([]string, len(vv))
		copy(vv2, vv)
		h2[k] = vv2
	}
	return h2
}

// Filter relevant response headers from
// the HEAD, GET http response. The function takes
// a list of headers which are filtered out and
// returned as a new http header.
func filterHeader(header http.Header, filterKeys []string) (filteredHeader http.Header) {
	filteredHeader = cloneHeader(header)
	for _, key := range filterKeys {
		filteredHeader.Del(key)
	}
	return filteredHeader
}

// regCred matches credential string in HTTP header
var regCred = regexp.MustCompile("Credential=([A-Z0-9]+)/")

// regCred matches signature string in HTTP header
var regSign = regexp.MustCompile("Signature=([[0-9a-f]+)")

// Redact out signature value from authorization string.
func redactSignature(origAuth string) string {
	if !strings.HasPrefix(origAuth, signV4Algorithm) {
		// Set a temporary redacted auth
		return "AWS **REDACTED**:**REDACTED**"
	}

	/// Signature V4 authorization header.

	// Strip out accessKeyID from:
	// Credential=<access-key-id>/<date>/<aws-region>/<aws-service>/aws4_request
	newAuth := regCred.ReplaceAllString(origAuth, "Credential=**REDACTED**/")

	// Strip out 256-bit signature from: Signature=<256-bit signature>
	return regSign.ReplaceAllString(newAuth, "Signature=**REDACTED**")
}

// Get default location returns the location based on the input
// URL `u`, if region override is provided then all location
// defaults to regionOverride.
//
// If no other cases match then the location is set to `us-east-1`
// as a last resort.
func getDefaultLocation(u url.URL, regionOverride string) (location string) {
	if regionOverride != "" {
		return regionOverride
	}
	if s3utils.IsAmazonChinaEndpoint(u) {
		return "cn-north-1"
	}
	if s3utils.IsAmazonGovCloudEndpoint(u) {
		return "us-gov-west-1"
	}
	// Default to location to 'us-east-1'.
	return "us-east-1"
}

var supportedHeaders = []string{
	"content-type",
	"cache-control",
	"content-encoding",
	"content-disposition",
	// Add more supported headers here.
}

// cseHeaders is list of client side encryption headers
var cseHeaders = []string{
	"X-Amz-Iv",
	"X-Amz-Key",
	"X-Amz-Matdesc",
}

// isStorageClassHeader returns true if the header is a supported storage class header
func isStorageClassHeader(headerKey string) bool {
	return strings.ToLower(amzStorageClass) == strings.ToLower(headerKey)
}

// isStandardHeader returns true if header is a supported header and not a custom header
func isStandardHeader(headerKey string) bool {
	key := strings.ToLower(headerKey)
	for _, header := range supportedHeaders {
		if strings.ToLower(header) == key {
			return true
		}
	}
	return false
}

// isCSEHeader returns true if header is a client side encryption header.
func isCSEHeader(headerKey string) bool {
	key := strings.ToLower(headerKey)
	for _, h := range cseHeaders {
		header := strings.ToLower(h)
		if (header == key) ||
			(("x-amz-meta-" + header) == key) {
			return true
		}
	}
	return false
}

// sseHeaders is list of server side encryption headers
var sseHeaders = []string{
	"x-amz-server-side-encryption",
	"x-amz-server-side-encryption-aws-kms-key-id",
	"x-amz-server-side-encryption-context",
	"x-amz-server-side-encryption-customer-algorithm",
	"x-amz-server-side-encryption-customer-key",
	"x-amz-server-side-encryption-customer-key-MD5",
}

// isSSEHeader returns true if header is a server side encryption header.
func isSSEHeader(headerKey string) bool {
	key := strings.ToLower(headerKey)
	for _, h := range sseHeaders {
		if strings.ToLower(h) == key {
			return true
		}
	}
	return false
}

// isAmzHeader returns true if header is a x-amz-meta-* or x-amz-acl header.
func isAmzHeader(headerKey string) bool {
	key := strings.ToLower(headerKey)

	return strings.HasPrefix(key, "x-amz-meta-") || key == "x-amz-acl"
}
