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

package s3utils

import (
	"bytes"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Sentinel URL is the default url value which is invalid.
var sentinelURL = url.URL{}

// IsValidDomain validates if input string is a valid domain name.
func IsValidDomain(host string) bool {
	// See RFC 1035, RFC 3696.
	host = strings.TrimSpace(host)
	if len(host) == 0 || len(host) > 255 {
		return false
	}
	// host cannot start or end with "-"
	if host[len(host)-1:] == "-" || host[:1] == "-" {
		return false
	}
	// host cannot start or end with "_"
	if host[len(host)-1:] == "_" || host[:1] == "_" {
		return false
	}
	// host cannot start or end with a "."
	if host[len(host)-1:] == "." || host[:1] == "." {
		return false
	}
	// All non alphanumeric characters are invalid.
	if strings.ContainsAny(host, "`~!@#$%^&*()+={}[]|\\\"';:><?/") {
		return false
	}
	// No need to regexp match, since the list is non-exhaustive.
	// We let it valid and fail later.
	return true
}

// IsValidIP parses input string for ip address validity.
func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IsVirtualHostSupported - verifies if bucketName can be part of
// virtual host. Currently only Amazon S3 and Google Cloud Storage
// would support this.
func IsVirtualHostSupported(endpointURL url.URL, bucketName string) bool {
	if endpointURL == sentinelURL {
		return false
	}
	// bucketName can be valid but '.' in the hostname will fail SSL
	// certificate validation. So do not use host-style for such buckets.
	if endpointURL.Scheme == "https" && strings.Contains(bucketName, ".") {
		return false
	}
	// Return true for all other cases
	return IsAmazonEndpoint(endpointURL) || IsGoogleEndpoint(endpointURL)
}

// AmazonS3Host - regular expression used to determine if an arg is s3 host.
var AmazonS3Host = regexp.MustCompile("^s3[.-]?(.*?)\\.amazonaws\\.com$")

// IsAmazonEndpoint - Match if it is exactly Amazon S3 endpoint.
func IsAmazonEndpoint(endpointURL url.URL) bool {
	if IsAmazonChinaEndpoint(endpointURL) {
		return true
	}
	if IsAmazonGovCloudEndpoint(endpointURL) {
		return true
	}
	return AmazonS3Host.MatchString(endpointURL.Host)
}

// IsAmazonGovCloudEndpoint - Match if it is exactly Amazon S3 GovCloud endpoint.
func IsAmazonGovCloudEndpoint(endpointURL url.URL) bool {
	if endpointURL == sentinelURL {
		return false
	}
	return (endpointURL.Host == "s3-us-gov-west-1.amazonaws.com" ||
		IsAmazonFIPSGovCloudEndpoint(endpointURL))
}

// IsAmazonFIPSGovCloudEndpoint - Match if it is exactly Amazon S3 FIPS GovCloud endpoint.
func IsAmazonFIPSGovCloudEndpoint(endpointURL url.URL) bool {
	if endpointURL == sentinelURL {
		return false
	}
	return endpointURL.Host == "s3-fips-us-gov-west-1.amazonaws.com"
}

// IsAmazonChinaEndpoint - Match if it is exactly Amazon S3 China endpoint.
// Customers who wish to use the new Beijing Region are required
// to sign up for a separate set of account credentials unique to
// the China (Beijing) Region. Customers with existing AWS credentials
// will not be able to access resources in the new Region, and vice versa.
// For more info https://aws.amazon.com/about-aws/whats-new/2013/12/18/announcing-the-aws-china-beijing-region/
func IsAmazonChinaEndpoint(endpointURL url.URL) bool {
	if endpointURL == sentinelURL {
		return false
	}
	return endpointURL.Host == "s3.cn-north-1.amazonaws.com.cn"
}

// IsGoogleEndpoint - Match if it is exactly Google cloud storage endpoint.
func IsGoogleEndpoint(endpointURL url.URL) bool {
	if endpointURL == sentinelURL {
		return false
	}
	return endpointURL.Host == "storage.googleapis.com"
}

// Expects ascii encoded strings - from output of urlEncodePath
func percentEncodeSlash(s string) string {
	return strings.Replace(s, "/", "%2F", -1)
}

// QueryEncode - encodes query values in their URL encoded form. In
// addition to the percent encoding performed by urlEncodePath() used
// here, it also percent encodes '/' (forward slash)
func QueryEncode(v url.Values) string {
	if v == nil {
		return ""
	}
	var buf bytes.Buffer
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vs := v[k]
		prefix := percentEncodeSlash(EncodePath(k)) + "="
		for _, v := range vs {
			if buf.Len() > 0 {
				buf.WriteByte('&')
			}
			buf.WriteString(prefix)
			buf.WriteString(percentEncodeSlash(EncodePath(v)))
		}
	}
	return buf.String()
}

// if object matches reserved string, no need to encode them
var reservedObjectNames = regexp.MustCompile("^[a-zA-Z0-9-_.~/]+$")

// EncodePath encode the strings from UTF-8 byte representations to HTML hex escape sequences
//
// This is necessary since regular url.Parse() and url.Encode() functions do not support UTF-8
// non english characters cannot be parsed due to the nature in which url.Encode() is written
//
// This function on the other hand is a direct replacement for url.Encode() technique to support
// pretty much every UTF-8 character.
func EncodePath(pathName string) string {
	if reservedObjectNames.MatchString(pathName) {
		return pathName
	}
	var encodedPathname string
	for _, s := range pathName {
		if 'A' <= s && s <= 'Z' || 'a' <= s && s <= 'z' || '0' <= s && s <= '9' { // §2.3 Unreserved characters (mark)
			encodedPathname = encodedPathname + string(s)
			continue
		}
		switch s {
		case '-', '_', '.', '~', '/': // §2.3 Unreserved characters (mark)
			encodedPathname = encodedPathname + string(s)
			continue
		default:
			len := utf8.RuneLen(s)
			if len < 0 {
				// if utf8 cannot convert return the same string as is
				return pathName
			}
			u := make([]byte, len)
			utf8.EncodeRune(u, s)
			for _, r := range u {
				hex := hex.EncodeToString([]byte{r})
				encodedPathname = encodedPathname + "%" + strings.ToUpper(hex)
			}
		}
	}
	return encodedPathname
}

// We support '.' with bucket names but we fallback to using path
// style requests instead for such buckets.
var (
	validBucketName       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9\.\-\_\:]{1,61}[A-Za-z0-9]$`)
	validBucketNameStrict = regexp.MustCompile(`^[a-z0-9][a-z0-9\.\-]{1,61}[a-z0-9]$`)
	ipAddress             = regexp.MustCompile(`^(\d+\.){3}\d+$`)
)

// Common checker for both stricter and basic validation.
func checkBucketNameCommon(bucketName string, strict bool) (err error) {
	if strings.TrimSpace(bucketName) == "" {
		return errors.New("Bucket name cannot be empty")
	}
	if len(bucketName) < 3 {
		return errors.New("Bucket name cannot be smaller than 3 characters")
	}
	if len(bucketName) > 63 {
		return errors.New("Bucket name cannot be greater than 63 characters")
	}
	if ipAddress.MatchString(bucketName) {
		return errors.New("Bucket name cannot be an ip address")
	}
	if strings.Contains(bucketName, "..") {
		return errors.New("Bucket name contains invalid characters")
	}
	if strict {
		if !validBucketNameStrict.MatchString(bucketName) {
			err = errors.New("Bucket name contains invalid characters")
		}
		return err
	}
	if !validBucketName.MatchString(bucketName) {
		err = errors.New("Bucket name contains invalid characters")
	}
	return err
}

// CheckValidBucketName - checks if we have a valid input bucket name.
func CheckValidBucketName(bucketName string) (err error) {
	return checkBucketNameCommon(bucketName, false)
}

// CheckValidBucketNameStrict - checks if we have a valid input bucket name.
// This is a stricter version.
// - http://docs.aws.amazon.com/AmazonS3/latest/dev/UsingBucket.html
func CheckValidBucketNameStrict(bucketName string) (err error) {
	return checkBucketNameCommon(bucketName, true)
}

// CheckValidObjectNamePrefix - checks if we have a valid input object name prefix.
//   - http://docs.aws.amazon.com/AmazonS3/latest/dev/UsingMetadata.html
func CheckValidObjectNamePrefix(objectName string) error {
	if len(objectName) > 1024 {
		return errors.New("Object name cannot be greater than 1024 characters")
	}
	if !utf8.ValidString(objectName) {
		return errors.New("Object name with non UTF-8 strings are not supported")
	}
	return nil
}

// CheckValidObjectName - checks if we have a valid input object name.
//   - http://docs.aws.amazon.com/AmazonS3/latest/dev/UsingMetadata.html
func CheckValidObjectName(objectName string) error {
	if strings.TrimSpace(objectName) == "" {
		return errors.New("Object name cannot be empty")
	}
	return CheckValidObjectNamePrefix(objectName)
}
