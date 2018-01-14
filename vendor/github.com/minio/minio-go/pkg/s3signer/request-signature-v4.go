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

package s3signer

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/pkg/s3utils"
)

// Signature and API related constants.
const (
	signV4Algorithm   = "AWS4-HMAC-SHA256"
	iso8601DateFormat = "20060102T150405Z"
	yyyymmdd          = "20060102"
)

///
/// Excerpts from @lsegal -
/// https://github.com/aws/aws-sdk-js/issues/659#issuecomment-120477258.
///
///  User-Agent:
///
///      This is ignored from signing because signing this causes
///      problems with generating pre-signed URLs (that are executed
///      by other agents) or when customers pass requests through
///      proxies, which may modify the user-agent.
///
///  Content-Length:
///
///      This is ignored from signing because generating a pre-signed
///      URL should not provide a content-length constraint,
///      specifically when vending a S3 pre-signed PUT URL. The
///      corollary to this is that when sending regular requests
///      (non-pre-signed), the signature contains a checksum of the
///      body, which implicitly validates the payload length (since
///      changing the number of bytes would change the checksum)
///      and therefore this header is not valuable in the signature.
///
///  Content-Type:
///
///      Signing this header causes quite a number of problems in
///      browser environments, where browsers like to modify and
///      normalize the content-type header in different ways. There is
///      more information on this in https://goo.gl/2E9gyy. Avoiding
///      this field simplifies logic and reduces the possibility of
///      future bugs.
///
///  Authorization:
///
///      Is skipped for obvious reasons
///
var v4IgnoredHeaders = map[string]bool{
	"Authorization":  true,
	"Content-Type":   true,
	"Content-Length": true,
	"User-Agent":     true,
}

// getSigningKey hmac seed to calculate final signature.
func getSigningKey(secret, loc string, t time.Time) []byte {
	date := sumHMAC([]byte("AWS4"+secret), []byte(t.Format(yyyymmdd)))
	location := sumHMAC(date, []byte(loc))
	service := sumHMAC(location, []byte("s3"))
	signingKey := sumHMAC(service, []byte("aws4_request"))
	return signingKey
}

// getSignature final signature in hexadecimal form.
func getSignature(signingKey []byte, stringToSign string) string {
	return hex.EncodeToString(sumHMAC(signingKey, []byte(stringToSign)))
}

// getScope generate a string of a specific date, an AWS region, and a
// service.
func getScope(location string, t time.Time) string {
	scope := strings.Join([]string{
		t.Format(yyyymmdd),
		location,
		"s3",
		"aws4_request",
	}, "/")
	return scope
}

// GetCredential generate a credential string.
func GetCredential(accessKeyID, location string, t time.Time) string {
	scope := getScope(location, t)
	return accessKeyID + "/" + scope
}

// getHashedPayload get the hexadecimal value of the SHA256 hash of
// the request payload.
func getHashedPayload(req http.Request) string {
	hashedPayload := req.Header.Get("X-Amz-Content-Sha256")
	if hashedPayload == "" {
		// Presign does not have a payload, use S3 recommended value.
		hashedPayload = unsignedPayload
	}
	return hashedPayload
}

// getCanonicalHeaders generate a list of request headers for
// signature.
func getCanonicalHeaders(req http.Request, ignoredHeaders map[string]bool) string {
	var headers []string
	vals := make(map[string][]string)
	for k, vv := range req.Header {
		if _, ok := ignoredHeaders[http.CanonicalHeaderKey(k)]; ok {
			continue // ignored header
		}
		headers = append(headers, strings.ToLower(k))
		vals[strings.ToLower(k)] = vv
	}
	headers = append(headers, "host")
	sort.Strings(headers)

	var buf bytes.Buffer
	// Save all the headers in canonical form <header>:<value> newline
	// separated for each header.
	for _, k := range headers {
		buf.WriteString(k)
		buf.WriteByte(':')
		switch {
		case k == "host":
			buf.WriteString(req.URL.Host)
			fallthrough
		default:
			for idx, v := range vals[k] {
				if idx > 0 {
					buf.WriteByte(',')
				}
				buf.WriteString(v)
			}
			buf.WriteByte('\n')
		}
	}
	return buf.String()
}

// getSignedHeaders generate all signed request headers.
// i.e lexically sorted, semicolon-separated list of lowercase
// request header names.
func getSignedHeaders(req http.Request, ignoredHeaders map[string]bool) string {
	var headers []string
	for k := range req.Header {
		if _, ok := ignoredHeaders[http.CanonicalHeaderKey(k)]; ok {
			continue // Ignored header found continue.
		}
		headers = append(headers, strings.ToLower(k))
	}
	headers = append(headers, "host")
	sort.Strings(headers)
	return strings.Join(headers, ";")
}

// getCanonicalRequest generate a canonical request of style.
//
// canonicalRequest =
//  <HTTPMethod>\n
//  <CanonicalURI>\n
//  <CanonicalQueryString>\n
//  <CanonicalHeaders>\n
//  <SignedHeaders>\n
//  <HashedPayload>
func getCanonicalRequest(req http.Request, ignoredHeaders map[string]bool) string {
	req.URL.RawQuery = strings.Replace(req.URL.Query().Encode(), "+", "%20", -1)
	canonicalRequest := strings.Join([]string{
		req.Method,
		s3utils.EncodePath(req.URL.Path),
		req.URL.RawQuery,
		getCanonicalHeaders(req, ignoredHeaders),
		getSignedHeaders(req, ignoredHeaders),
		getHashedPayload(req),
	}, "\n")
	return canonicalRequest
}

// getStringToSign a string based on selected query values.
func getStringToSignV4(t time.Time, location, canonicalRequest string) string {
	stringToSign := signV4Algorithm + "\n" + t.Format(iso8601DateFormat) + "\n"
	stringToSign = stringToSign + getScope(location, t) + "\n"
	stringToSign = stringToSign + hex.EncodeToString(sum256([]byte(canonicalRequest)))
	return stringToSign
}

// PreSignV4 presign the request, in accordance with
// http://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html.
func PreSignV4(req http.Request, accessKeyID, secretAccessKey, sessionToken, location string, expires int64) *http.Request {
	// Presign is not needed for anonymous credentials.
	if accessKeyID == "" || secretAccessKey == "" {
		return &req
	}

	// Initial time.
	t := time.Now().UTC()

	// Get credential string.
	credential := GetCredential(accessKeyID, location, t)

	// Get all signed headers.
	signedHeaders := getSignedHeaders(req, v4IgnoredHeaders)

	// Set URL query.
	query := req.URL.Query()
	query.Set("X-Amz-Algorithm", signV4Algorithm)
	query.Set("X-Amz-Date", t.Format(iso8601DateFormat))
	query.Set("X-Amz-Expires", strconv.FormatInt(expires, 10))
	query.Set("X-Amz-SignedHeaders", signedHeaders)
	query.Set("X-Amz-Credential", credential)
	// Set session token if available.
	if sessionToken != "" {
		query.Set("X-Amz-Security-Token", sessionToken)
	}
	req.URL.RawQuery = query.Encode()

	// Get canonical request.
	canonicalRequest := getCanonicalRequest(req, v4IgnoredHeaders)

	// Get string to sign from canonical request.
	stringToSign := getStringToSignV4(t, location, canonicalRequest)

	// Gext hmac signing key.
	signingKey := getSigningKey(secretAccessKey, location, t)

	// Calculate signature.
	signature := getSignature(signingKey, stringToSign)

	// Add signature header to RawQuery.
	req.URL.RawQuery += "&X-Amz-Signature=" + signature

	return &req
}

// PostPresignSignatureV4 - presigned signature for PostPolicy
// requests.
func PostPresignSignatureV4(policyBase64 string, t time.Time, secretAccessKey, location string) string {
	// Get signining key.
	signingkey := getSigningKey(secretAccessKey, location, t)
	// Calculate signature.
	signature := getSignature(signingkey, policyBase64)
	return signature
}

// SignV4 sign the request before Do(), in accordance with
// http://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-authenticating-requests.html.
func SignV4(req http.Request, accessKeyID, secretAccessKey, sessionToken, location string) *http.Request {
	// Signature calculation is not needed for anonymous credentials.
	if accessKeyID == "" || secretAccessKey == "" {
		return &req
	}

	// Initial time.
	t := time.Now().UTC()

	// Set x-amz-date.
	req.Header.Set("X-Amz-Date", t.Format(iso8601DateFormat))

	// Set session token if available.
	if sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", sessionToken)
	}

	// Get canonical request.
	canonicalRequest := getCanonicalRequest(req, v4IgnoredHeaders)

	// Get string to sign from canonical request.
	stringToSign := getStringToSignV4(t, location, canonicalRequest)

	// Get hmac signing key.
	signingKey := getSigningKey(secretAccessKey, location, t)

	// Get credential string.
	credential := GetCredential(accessKeyID, location, t)

	// Get all signed headers.
	signedHeaders := getSignedHeaders(req, v4IgnoredHeaders)

	// Calculate signature.
	signature := getSignature(signingKey, stringToSign)

	// If regular request, construct the final authorization header.
	parts := []string{
		signV4Algorithm + " Credential=" + credential,
		"SignedHeaders=" + signedHeaders,
		"Signature=" + signature,
	}

	// Set authorization header.
	auth := strings.Join(parts, ", ")
	req.Header.Set("Authorization", auth)

	return &req
}
