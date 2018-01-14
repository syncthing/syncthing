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
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/pkg/s3utils"
)

// Signature and API related constants.
const (
	signV2Algorithm = "AWS"
)

// Encode input URL path to URL encoded path.
func encodeURL2Path(u *url.URL) (path string) {
	// Encode URL path.
	if isS3, _ := filepath.Match("*.s3*.amazonaws.com", u.Host); isS3 {
		bucketName := u.Host[:strings.LastIndex(u.Host, ".s3")]
		path = "/" + bucketName
		path += u.Path
		path = s3utils.EncodePath(path)
		return
	}
	if strings.HasSuffix(u.Host, ".storage.googleapis.com") {
		path = "/" + strings.TrimSuffix(u.Host, ".storage.googleapis.com")
		path += u.Path
		path = s3utils.EncodePath(path)
		return
	}
	path = s3utils.EncodePath(u.Path)
	return
}

// PreSignV2 - presign the request in following style.
// https://${S3_BUCKET}.s3.amazonaws.com/${S3_OBJECT}?AWSAccessKeyId=${S3_ACCESS_KEY}&Expires=${TIMESTAMP}&Signature=${SIGNATURE}.
func PreSignV2(req http.Request, accessKeyID, secretAccessKey string, expires int64) *http.Request {
	// Presign is not needed for anonymous credentials.
	if accessKeyID == "" || secretAccessKey == "" {
		return &req
	}

	d := time.Now().UTC()
	// Find epoch expires when the request will expire.
	epochExpires := d.Unix() + expires

	// Add expires header if not present.
	if expiresStr := req.Header.Get("Expires"); expiresStr == "" {
		req.Header.Set("Expires", strconv.FormatInt(epochExpires, 10))
	}

	// Get presigned string to sign.
	stringToSign := preStringToSignV2(req)
	hm := hmac.New(sha1.New, []byte(secretAccessKey))
	hm.Write([]byte(stringToSign))

	// Calculate signature.
	signature := base64.StdEncoding.EncodeToString(hm.Sum(nil))

	query := req.URL.Query()
	// Handle specially for Google Cloud Storage.
	if strings.Contains(req.URL.Host, ".storage.googleapis.com") {
		query.Set("GoogleAccessId", accessKeyID)
	} else {
		query.Set("AWSAccessKeyId", accessKeyID)
	}

	// Fill in Expires for presigned query.
	query.Set("Expires", strconv.FormatInt(epochExpires, 10))

	// Encode query and save.
	req.URL.RawQuery = s3utils.QueryEncode(query)

	// Save signature finally.
	req.URL.RawQuery += "&Signature=" + s3utils.EncodePath(signature)

	// Return.
	return &req
}

// PostPresignSignatureV2 - presigned signature for PostPolicy
// request.
func PostPresignSignatureV2(policyBase64, secretAccessKey string) string {
	hm := hmac.New(sha1.New, []byte(secretAccessKey))
	hm.Write([]byte(policyBase64))
	signature := base64.StdEncoding.EncodeToString(hm.Sum(nil))
	return signature
}

// Authorization = "AWS" + " " + AWSAccessKeyId + ":" + Signature;
// Signature = Base64( HMAC-SHA1( YourSecretAccessKeyID, UTF-8-Encoding-Of( StringToSign ) ) );
//
// StringToSign = HTTP-Verb + "\n" +
//  	Content-Md5 + "\n" +
//  	Content-Type + "\n" +
//  	Date + "\n" +
//  	CanonicalizedProtocolHeaders +
//  	CanonicalizedResource;
//
// CanonicalizedResource = [ "/" + Bucket ] +
//  	<HTTP-Request-URI, from the protocol name up to the query string> +
//  	[ subresource, if present. For example "?acl", "?location", "?logging", or "?torrent"];
//
// CanonicalizedProtocolHeaders = <described below>

// SignV2 sign the request before Do() (AWS Signature Version 2).
func SignV2(req http.Request, accessKeyID, secretAccessKey string) *http.Request {
	// Signature calculation is not needed for anonymous credentials.
	if accessKeyID == "" || secretAccessKey == "" {
		return &req
	}

	// Initial time.
	d := time.Now().UTC()

	// Add date if not present.
	if date := req.Header.Get("Date"); date == "" {
		req.Header.Set("Date", d.Format(http.TimeFormat))
	}

	// Calculate HMAC for secretAccessKey.
	stringToSign := stringToSignV2(req)
	hm := hmac.New(sha1.New, []byte(secretAccessKey))
	hm.Write([]byte(stringToSign))

	// Prepare auth header.
	authHeader := new(bytes.Buffer)
	authHeader.WriteString(fmt.Sprintf("%s %s:", signV2Algorithm, accessKeyID))
	encoder := base64.NewEncoder(base64.StdEncoding, authHeader)
	encoder.Write(hm.Sum(nil))
	encoder.Close()

	// Set Authorization header.
	req.Header.Set("Authorization", authHeader.String())

	return &req
}

// From the Amazon docs:
//
// StringToSign = HTTP-Verb + "\n" +
// 	 Content-Md5 + "\n" +
//	 Content-Type + "\n" +
//	 Expires + "\n" +
//	 CanonicalizedProtocolHeaders +
//	 CanonicalizedResource;
func preStringToSignV2(req http.Request) string {
	buf := new(bytes.Buffer)
	// Write standard headers.
	writePreSignV2Headers(buf, req)
	// Write canonicalized protocol headers if any.
	writeCanonicalizedHeaders(buf, req)
	// Write canonicalized Query resources if any.
	writeCanonicalizedResource(buf, req)
	return buf.String()
}

// writePreSignV2Headers - write preSign v2 required headers.
func writePreSignV2Headers(buf *bytes.Buffer, req http.Request) {
	buf.WriteString(req.Method + "\n")
	buf.WriteString(req.Header.Get("Content-Md5") + "\n")
	buf.WriteString(req.Header.Get("Content-Type") + "\n")
	buf.WriteString(req.Header.Get("Expires") + "\n")
}

// From the Amazon docs:
//
// StringToSign = HTTP-Verb + "\n" +
// 	 Content-Md5 + "\n" +
//	 Content-Type + "\n" +
//	 Date + "\n" +
//	 CanonicalizedProtocolHeaders +
//	 CanonicalizedResource;
func stringToSignV2(req http.Request) string {
	buf := new(bytes.Buffer)
	// Write standard headers.
	writeSignV2Headers(buf, req)
	// Write canonicalized protocol headers if any.
	writeCanonicalizedHeaders(buf, req)
	// Write canonicalized Query resources if any.
	writeCanonicalizedResource(buf, req)
	return buf.String()
}

// writeSignV2Headers - write signV2 required headers.
func writeSignV2Headers(buf *bytes.Buffer, req http.Request) {
	buf.WriteString(req.Method + "\n")
	buf.WriteString(req.Header.Get("Content-Md5") + "\n")
	buf.WriteString(req.Header.Get("Content-Type") + "\n")
	buf.WriteString(req.Header.Get("Date") + "\n")
}

// writeCanonicalizedHeaders - write canonicalized headers.
func writeCanonicalizedHeaders(buf *bytes.Buffer, req http.Request) {
	var protoHeaders []string
	vals := make(map[string][]string)
	for k, vv := range req.Header {
		// All the AMZ headers should be lowercase
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz") {
			protoHeaders = append(protoHeaders, lk)
			vals[lk] = vv
		}
	}
	sort.Strings(protoHeaders)
	for _, k := range protoHeaders {
		buf.WriteString(k)
		buf.WriteByte(':')
		for idx, v := range vals[k] {
			if idx > 0 {
				buf.WriteByte(',')
			}
			if strings.Contains(v, "\n") {
				// TODO: "Unfold" long headers that
				// span multiple lines (as allowed by
				// RFC 2616, section 4.2) by replacing
				// the folding white-space (including
				// new-line) by a single space.
				buf.WriteString(v)
			} else {
				buf.WriteString(v)
			}
		}
		buf.WriteByte('\n')
	}
}

// AWS S3 Signature V2 calculation rule is give here:
// http://docs.aws.amazon.com/AmazonS3/latest/dev/RESTAuthentication.html#RESTAuthenticationStringToSign

// Whitelist resource list that will be used in query string for signature-V2 calculation.
// The list should be alphabetically sorted
var resourceList = []string{
	"acl",
	"delete",
	"lifecycle",
	"location",
	"logging",
	"notification",
	"partNumber",
	"policy",
	"requestPayment",
	"response-cache-control",
	"response-content-disposition",
	"response-content-encoding",
	"response-content-language",
	"response-content-type",
	"response-expires",
	"torrent",
	"uploadId",
	"uploads",
	"versionId",
	"versioning",
	"versions",
	"website",
}

// From the Amazon docs:
//
// CanonicalizedResource = [ "/" + Bucket ] +
// 	  <HTTP-Request-URI, from the protocol name up to the query string> +
// 	  [ sub-resource, if present. For example "?acl", "?location", "?logging", or "?torrent"];
func writeCanonicalizedResource(buf *bytes.Buffer, req http.Request) {
	// Save request URL.
	requestURL := req.URL
	// Get encoded URL path.
	buf.WriteString(encodeURL2Path(requestURL))
	if requestURL.RawQuery != "" {
		var n int
		vals, _ := url.ParseQuery(requestURL.RawQuery)
		// Verify if any sub resource queries are present, if yes
		// canonicallize them.
		for _, resource := range resourceList {
			if vv, ok := vals[resource]; ok && len(vv) > 0 {
				n++
				// First element
				switch n {
				case 1:
					buf.WriteByte('?')
				// The rest
				default:
					buf.WriteByte('&')
				}
				buf.WriteString(resource)
				// Request parameters
				if len(vv[0]) > 0 {
					buf.WriteByte('=')
					buf.WriteString(vv[0])
				}
			}
		}
	}
}
