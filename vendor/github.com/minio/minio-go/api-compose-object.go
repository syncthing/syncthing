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

package minio

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/pkg/s3utils"
)

// SSEInfo - represents Server-Side-Encryption parameters specified by
// a user.
type SSEInfo struct {
	key  []byte
	algo string
}

// NewSSEInfo - specifies (binary or un-encoded) encryption key and
// algorithm name. If algo is empty, it defaults to "AES256". Ref:
// https://docs.aws.amazon.com/AmazonS3/latest/dev/ServerSideEncryptionCustomerKeys.html
func NewSSEInfo(key []byte, algo string) SSEInfo {
	if algo == "" {
		algo = "AES256"
	}
	return SSEInfo{key, algo}
}

// internal method that computes SSE-C headers
func (s *SSEInfo) getSSEHeaders(isCopySource bool) map[string]string {
	if s == nil {
		return nil
	}

	cs := ""
	if isCopySource {
		cs = "copy-source-"
	}
	return map[string]string{
		"x-amz-" + cs + "server-side-encryption-customer-algorithm": s.algo,
		"x-amz-" + cs + "server-side-encryption-customer-key":       base64.StdEncoding.EncodeToString(s.key),
		"x-amz-" + cs + "server-side-encryption-customer-key-MD5":   sumMD5Base64(s.key),
	}
}

// GetSSEHeaders - computes and returns headers for SSE-C as key-value
// pairs. They can be set as metadata in PutObject* requests (for
// encryption) or be set as request headers in `Core.GetObject` (for
// decryption).
func (s *SSEInfo) GetSSEHeaders() map[string]string {
	return s.getSSEHeaders(false)
}

// DestinationInfo - type with information about the object to be
// created via server-side copy requests, using the Compose API.
type DestinationInfo struct {
	bucket, object string

	// key for encrypting destination
	encryption *SSEInfo

	// if no user-metadata is provided, it is copied from source
	// (when there is only once source object in the compose
	// request)
	userMetadata map[string]string
}

// NewDestinationInfo - creates a compose-object/copy-source
// destination info object.
//
// `encSSEC` is the key info for server-side-encryption with customer
// provided key. If it is nil, no encryption is performed.
//
// `userMeta` is the user-metadata key-value pairs to be set on the
// destination. The keys are automatically prefixed with `x-amz-meta-`
// if needed. If nil is passed, and if only a single source (of any
// size) is provided in the ComposeObject call, then metadata from the
// source is copied to the destination.
func NewDestinationInfo(bucket, object string, encryptSSEC *SSEInfo,
	userMeta map[string]string) (d DestinationInfo, err error) {

	// Input validation.
	if err = s3utils.CheckValidBucketName(bucket); err != nil {
		return d, err
	}
	if err = s3utils.CheckValidObjectName(object); err != nil {
		return d, err
	}

	// Process custom-metadata to remove a `x-amz-meta-` prefix if
	// present and validate that keys are distinct (after this
	// prefix removal).
	m := make(map[string]string)
	for k, v := range userMeta {
		if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") {
			k = k[len("x-amz-meta-"):]
		}
		if _, ok := m[k]; ok {
			return d, ErrInvalidArgument(fmt.Sprintf("Cannot add both %s and x-amz-meta-%s keys as custom metadata", k, k))
		}
		m[k] = v
	}

	return DestinationInfo{
		bucket:       bucket,
		object:       object,
		encryption:   encryptSSEC,
		userMetadata: m,
	}, nil
}

// getUserMetaHeadersMap - construct appropriate key-value pairs to send
// as headers from metadata map to pass into copy-object request. For
// single part copy-object (i.e. non-multipart object), enable the
// withCopyDirectiveHeader to set the `x-amz-metadata-directive` to
// `REPLACE`, so that metadata headers from the source are not copied
// over.
func (d *DestinationInfo) getUserMetaHeadersMap(withCopyDirectiveHeader bool) map[string]string {
	if len(d.userMetadata) == 0 {
		return nil
	}
	r := make(map[string]string)
	if withCopyDirectiveHeader {
		r["x-amz-metadata-directive"] = "REPLACE"
	}
	for k, v := range d.userMetadata {
		r["x-amz-meta-"+k] = v
	}
	return r
}

// SourceInfo - represents a source object to be copied, using
// server-side copying APIs.
type SourceInfo struct {
	bucket, object string

	start, end int64

	decryptKey *SSEInfo
	// Headers to send with the upload-part-copy request involving
	// this source object.
	Headers http.Header
}

// NewSourceInfo - create a compose-object/copy-object source info
// object.
//
// `decryptSSEC` is the decryption key using server-side-encryption
// with customer provided key. It may be nil if the source is not
// encrypted.
func NewSourceInfo(bucket, object string, decryptSSEC *SSEInfo) SourceInfo {
	r := SourceInfo{
		bucket:     bucket,
		object:     object,
		start:      -1, // range is unspecified by default
		decryptKey: decryptSSEC,
		Headers:    make(http.Header),
	}

	// Set the source header
	r.Headers.Set("x-amz-copy-source", s3utils.EncodePath(bucket+"/"+object))

	// Assemble decryption headers for upload-part-copy request
	for k, v := range decryptSSEC.getSSEHeaders(true) {
		r.Headers.Set(k, v)
	}

	return r
}

// SetRange - Set the start and end offset of the source object to be
// copied. If this method is not called, the whole source object is
// copied.
func (s *SourceInfo) SetRange(start, end int64) error {
	if start > end || start < 0 {
		return ErrInvalidArgument("start must be non-negative, and start must be at most end.")
	}
	// Note that 0 <= start <= end
	s.start, s.end = start, end
	return nil
}

// SetMatchETagCond - Set ETag match condition. The object is copied
// only if the etag of the source matches the value given here.
func (s *SourceInfo) SetMatchETagCond(etag string) error {
	if etag == "" {
		return ErrInvalidArgument("ETag cannot be empty.")
	}
	s.Headers.Set("x-amz-copy-source-if-match", etag)
	return nil
}

// SetMatchETagExceptCond - Set the ETag match exception
// condition. The object is copied only if the etag of the source is
// not the value given here.
func (s *SourceInfo) SetMatchETagExceptCond(etag string) error {
	if etag == "" {
		return ErrInvalidArgument("ETag cannot be empty.")
	}
	s.Headers.Set("x-amz-copy-source-if-none-match", etag)
	return nil
}

// SetModifiedSinceCond - Set the modified since condition.
func (s *SourceInfo) SetModifiedSinceCond(modTime time.Time) error {
	if modTime.IsZero() {
		return ErrInvalidArgument("Input time cannot be 0.")
	}
	s.Headers.Set("x-amz-copy-source-if-modified-since", modTime.Format(http.TimeFormat))
	return nil
}

// SetUnmodifiedSinceCond - Set the unmodified since condition.
func (s *SourceInfo) SetUnmodifiedSinceCond(modTime time.Time) error {
	if modTime.IsZero() {
		return ErrInvalidArgument("Input time cannot be 0.")
	}
	s.Headers.Set("x-amz-copy-source-if-unmodified-since", modTime.Format(http.TimeFormat))
	return nil
}

// Helper to fetch size and etag of an object using a StatObject call.
func (s *SourceInfo) getProps(c Client) (size int64, etag string, userMeta map[string]string, err error) {
	// Get object info - need size and etag here. Also, decryption
	// headers are added to the stat request if given.
	var objInfo ObjectInfo
	opts := StatObjectOptions{}
	for k, v := range s.decryptKey.getSSEHeaders(false) {
		opts.Set(k, v)
	}
	objInfo, err = c.statObject(context.Background(), s.bucket, s.object, opts)
	if err != nil {
		err = ErrInvalidArgument(fmt.Sprintf("Could not stat object - %s/%s: %v", s.bucket, s.object, err))
	} else {
		size = objInfo.Size
		etag = objInfo.ETag
		userMeta = make(map[string]string)
		for k, v := range objInfo.Metadata {
			if strings.HasPrefix(k, "x-amz-meta-") {
				if len(v) > 0 {
					userMeta[k] = v[0]
				}
			}
		}
	}
	return
}

// Low level implementation of CopyObject API, supports only upto 5GiB worth of copy.
func (c Client) copyObjectDo(ctx context.Context, srcBucket, srcObject, destBucket, destObject string,
	metadata map[string]string) (ObjectInfo, error) {

	// Build headers.
	headers := make(http.Header)

	// Set all the metadata headers.
	for k, v := range metadata {
		headers.Set(k, v)
	}

	// Set the source header
	headers.Set("x-amz-copy-source", s3utils.EncodePath(srcBucket+"/"+srcObject))

	// Send upload-part-copy request
	resp, err := c.executeMethod(ctx, "PUT", requestMetadata{
		bucketName:   destBucket,
		objectName:   destObject,
		customHeader: headers,
	})
	defer closeResponse(resp)
	if err != nil {
		return ObjectInfo{}, err
	}

	// Check if we got an error response.
	if resp.StatusCode != http.StatusOK {
		return ObjectInfo{}, httpRespToErrorResponse(resp, srcBucket, srcObject)
	}

	cpObjRes := copyObjectResult{}
	err = xmlDecoder(resp.Body, &cpObjRes)
	if err != nil {
		return ObjectInfo{}, err
	}

	objInfo := ObjectInfo{
		Key:          destObject,
		ETag:         strings.Trim(cpObjRes.ETag, "\""),
		LastModified: cpObjRes.LastModified,
	}
	return objInfo, nil
}

func (c Client) copyObjectPartDo(ctx context.Context, srcBucket, srcObject, destBucket, destObject string, uploadID string,
	partID int, startOffset int64, length int64, metadata map[string]string) (p CompletePart, err error) {

	headers := make(http.Header)

	// Set source
	headers.Set("x-amz-copy-source", s3utils.EncodePath(srcBucket+"/"+srcObject))

	if startOffset < 0 {
		return p, ErrInvalidArgument("startOffset must be non-negative")
	}

	if length >= 0 {
		headers.Set("x-amz-copy-source-range", fmt.Sprintf("bytes=%d-%d", startOffset, startOffset+length-1))
	}

	for k, v := range metadata {
		headers.Set(k, v)
	}

	queryValues := make(url.Values)
	queryValues.Set("partNumber", strconv.Itoa(partID))
	queryValues.Set("uploadId", uploadID)

	resp, err := c.executeMethod(ctx, "PUT", requestMetadata{
		bucketName:   destBucket,
		objectName:   destObject,
		customHeader: headers,
		queryValues:  queryValues,
	})
	defer closeResponse(resp)
	if err != nil {
		return
	}

	// Check if we got an error response.
	if resp.StatusCode != http.StatusOK {
		return p, httpRespToErrorResponse(resp, destBucket, destObject)
	}

	// Decode copy-part response on success.
	cpObjRes := copyObjectResult{}
	err = xmlDecoder(resp.Body, &cpObjRes)
	if err != nil {
		return p, err
	}
	p.PartNumber, p.ETag = partID, cpObjRes.ETag
	return p, nil
}

// uploadPartCopy - helper function to create a part in a multipart
// upload via an upload-part-copy request
// https://docs.aws.amazon.com/AmazonS3/latest/API/mpUploadUploadPartCopy.html
func (c Client) uploadPartCopy(ctx context.Context, bucket, object, uploadID string, partNumber int,
	headers http.Header) (p CompletePart, err error) {

	// Build query parameters
	urlValues := make(url.Values)
	urlValues.Set("partNumber", strconv.Itoa(partNumber))
	urlValues.Set("uploadId", uploadID)

	// Send upload-part-copy request
	resp, err := c.executeMethod(ctx, "PUT", requestMetadata{
		bucketName:   bucket,
		objectName:   object,
		customHeader: headers,
		queryValues:  urlValues,
	})
	defer closeResponse(resp)
	if err != nil {
		return p, err
	}

	// Check if we got an error response.
	if resp.StatusCode != http.StatusOK {
		return p, httpRespToErrorResponse(resp, bucket, object)
	}

	// Decode copy-part response on success.
	cpObjRes := copyObjectResult{}
	err = xmlDecoder(resp.Body, &cpObjRes)
	if err != nil {
		return p, err
	}
	p.PartNumber, p.ETag = partNumber, cpObjRes.ETag
	return p, nil
}

// ComposeObject - creates an object using server-side copying of
// existing objects. It takes a list of source objects (with optional
// offsets) and concatenates them into a new object using only
// server-side copying operations.
func (c Client) ComposeObject(dst DestinationInfo, srcs []SourceInfo) error {
	if len(srcs) < 1 || len(srcs) > maxPartsCount {
		return ErrInvalidArgument("There must be as least one and up to 10000 source objects.")
	}
	ctx := context.Background()
	srcSizes := make([]int64, len(srcs))
	var totalSize, size, totalParts int64
	var srcUserMeta map[string]string
	var etag string
	var err error
	for i, src := range srcs {
		size, etag, srcUserMeta, err = src.getProps(c)
		if err != nil {
			return err
		}

		// Error out if client side encryption is used in this source object when
		// more than one source objects are given.
		if len(srcs) > 1 && src.Headers.Get("x-amz-meta-x-amz-key") != "" {
			return ErrInvalidArgument(
				fmt.Sprintf("Client side encryption is used in source object %s/%s", src.bucket, src.object))
		}

		// Since we did a HEAD to get size, we use the ETag
		// value to make sure the object has not changed by
		// the time we perform the copy. This is done, only if
		// the user has not set their own ETag match
		// condition.
		if src.Headers.Get("x-amz-copy-source-if-match") == "" {
			src.SetMatchETagCond(etag)
		}

		// Check if a segment is specified, and if so, is the
		// segment within object bounds?
		if src.start != -1 {
			// Since range is specified,
			//    0 <= src.start <= src.end
			// so only invalid case to check is:
			if src.end >= size {
				return ErrInvalidArgument(
					fmt.Sprintf("SourceInfo %d has invalid segment-to-copy [%d, %d] (size is %d)",
						i, src.start, src.end, size))
			}
			size = src.end - src.start + 1
		}

		// Only the last source may be less than `absMinPartSize`
		if size < absMinPartSize && i < len(srcs)-1 {
			return ErrInvalidArgument(
				fmt.Sprintf("SourceInfo %d is too small (%d) and it is not the last part", i, size))
		}

		// Is data to copy too large?
		totalSize += size
		if totalSize > maxMultipartPutObjectSize {
			return ErrInvalidArgument(fmt.Sprintf("Cannot compose an object of size %d (> 5TiB)", totalSize))
		}

		// record source size
		srcSizes[i] = size

		// calculate parts needed for current source
		totalParts += partsRequired(size)
		// Do we need more parts than we are allowed?
		if totalParts > maxPartsCount {
			return ErrInvalidArgument(fmt.Sprintf(
				"Your proposed compose object requires more than %d parts", maxPartsCount))
		}
	}

	// Single source object case (i.e. when only one source is
	// involved, it is being copied wholly and at most 5GiB in
	// size).
	if totalParts == 1 && srcs[0].start == -1 && totalSize <= maxPartSize {
		h := srcs[0].Headers
		// Add destination encryption headers
		for k, v := range dst.encryption.getSSEHeaders(false) {
			h.Set(k, v)
		}

		// If no user metadata is specified (and so, the
		// for-loop below is not entered), metadata from the
		// source is copied to the destination (due to
		// single-part copy-object PUT request behaviour).
		for k, v := range dst.getUserMetaHeadersMap(true) {
			h.Set(k, v)
		}

		// Send copy request
		resp, err := c.executeMethod(ctx, "PUT", requestMetadata{
			bucketName:   dst.bucket,
			objectName:   dst.object,
			customHeader: h,
		})
		defer closeResponse(resp)
		if err != nil {
			return err
		}
		// Check if we got an error response.
		if resp.StatusCode != http.StatusOK {
			return httpRespToErrorResponse(resp, dst.bucket, dst.object)
		}

		// Return nil on success.
		return nil
	}

	// Now, handle multipart-copy cases.

	// 1. Initiate a new multipart upload.

	// Set user-metadata on the destination object. If no
	// user-metadata is specified, and there is only one source,
	// (only) then metadata from source is copied.
	userMeta := dst.getUserMetaHeadersMap(false)
	metaMap := userMeta
	if len(userMeta) == 0 && len(srcs) == 1 {
		metaMap = srcUserMeta
	}
	metaHeaders := make(map[string]string)
	for k, v := range metaMap {
		metaHeaders[k] = v
	}
	uploadID, err := c.newUploadID(ctx, dst.bucket, dst.object, PutObjectOptions{UserMetadata: metaHeaders})
	if err != nil {
		return err
	}

	// 2. Perform copy part uploads
	objParts := []CompletePart{}
	partIndex := 1
	for i, src := range srcs {
		h := src.Headers
		// Add destination encryption headers
		for k, v := range dst.encryption.getSSEHeaders(false) {
			h.Set(k, v)
		}

		// calculate start/end indices of parts after
		// splitting.
		startIdx, endIdx := calculateEvenSplits(srcSizes[i], src)
		for j, start := range startIdx {
			end := endIdx[j]

			// Add (or reset) source range header for
			// upload part copy request.
			h.Set("x-amz-copy-source-range",
				fmt.Sprintf("bytes=%d-%d", start, end))

			// make upload-part-copy request
			complPart, err := c.uploadPartCopy(ctx, dst.bucket,
				dst.object, uploadID, partIndex, h)
			if err != nil {
				return err
			}
			objParts = append(objParts, complPart)
			partIndex++
		}
	}

	// 3. Make final complete-multipart request.
	_, err = c.completeMultipartUpload(ctx, dst.bucket, dst.object, uploadID,
		completeMultipartUpload{Parts: objParts})
	if err != nil {
		return err
	}
	return nil
}

// partsRequired is ceiling(size / copyPartSize)
func partsRequired(size int64) int64 {
	r := size / copyPartSize
	if size%copyPartSize > 0 {
		r++
	}
	return r
}

// calculateEvenSplits - computes splits for a source and returns
// start and end index slices. Splits happen evenly to be sure that no
// part is less than 5MiB, as that could fail the multipart request if
// it is not the last part.
func calculateEvenSplits(size int64, src SourceInfo) (startIndex, endIndex []int64) {
	if size == 0 {
		return
	}

	reqParts := partsRequired(size)
	startIndex = make([]int64, reqParts)
	endIndex = make([]int64, reqParts)
	// Compute number of required parts `k`, as:
	//
	// k = ceiling(size / copyPartSize)
	//
	// Now, distribute the `size` bytes in the source into
	// k parts as evenly as possible:
	//
	// r parts sized (q+1) bytes, and
	// (k - r) parts sized q bytes, where
	//
	// size = q * k + r (by simple division of size by k,
	// so that 0 <= r < k)
	//
	start := src.start
	if start == -1 {
		start = 0
	}
	quot, rem := size/reqParts, size%reqParts
	nextStart := start
	for j := int64(0); j < reqParts; j++ {
		curPartSize := quot
		if j < rem {
			curPartSize++
		}

		cStart := nextStart
		cEnd := cStart + curPartSize - 1
		nextStart = cEnd + 1

		startIndex[j], endIndex[j] = cStart, cEnd
	}
	return
}
