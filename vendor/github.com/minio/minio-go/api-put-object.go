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
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"sort"

	"github.com/minio/minio-go/pkg/encrypt"
	"github.com/minio/minio-go/pkg/s3utils"
)

// PutObjectOptions represents options specified by user for PutObject call
type PutObjectOptions struct {
	UserMetadata       map[string]string
	Progress           io.Reader
	ContentType        string
	ContentEncoding    string
	ContentDisposition string
	CacheControl       string
	EncryptMaterials   encrypt.Materials
	NumThreads         uint
	StorageClass       string
}

// getNumThreads - gets the number of threads to be used in the multipart
// put object operation
func (opts PutObjectOptions) getNumThreads() (numThreads int) {
	if opts.NumThreads > 0 {
		numThreads = int(opts.NumThreads)
	} else {
		numThreads = totalWorkers
	}
	return
}

// Header - constructs the headers from metadata entered by user in
// PutObjectOptions struct
func (opts PutObjectOptions) Header() (header http.Header) {
	header = make(http.Header)

	if opts.ContentType != "" {
		header["Content-Type"] = []string{opts.ContentType}
	} else {
		header["Content-Type"] = []string{"application/octet-stream"}
	}
	if opts.ContentEncoding != "" {
		header["Content-Encoding"] = []string{opts.ContentEncoding}
	}
	if opts.ContentDisposition != "" {
		header["Content-Disposition"] = []string{opts.ContentDisposition}
	}
	if opts.CacheControl != "" {
		header["Cache-Control"] = []string{opts.CacheControl}
	}
	if opts.EncryptMaterials != nil {
		header[amzHeaderIV] = []string{opts.EncryptMaterials.GetIV()}
		header[amzHeaderKey] = []string{opts.EncryptMaterials.GetKey()}
		header[amzHeaderMatDesc] = []string{opts.EncryptMaterials.GetDesc()}
	}
	if opts.StorageClass != "" {
		header[amzStorageClass] = []string{opts.StorageClass}
	}
	for k, v := range opts.UserMetadata {
		if !isAmzHeader(k) && !isStandardHeader(k) && !isSSEHeader(k) && !isStorageClassHeader(k) {
			header["X-Amz-Meta-"+k] = []string{v}
		} else {
			header[k] = []string{v}
		}
	}
	return
}

// validate() checks if the UserMetadata map has standard headers or client side
// encryption headers and raises an error if so.
func (opts PutObjectOptions) validate() (err error) {
	for k := range opts.UserMetadata {
		if isStandardHeader(k) || isCSEHeader(k) || isStorageClassHeader(k) {
			return ErrInvalidArgument(k + " unsupported request parameter for user defined metadata from minio-go")
		}
	}
	return nil
}

// completedParts is a collection of parts sortable by their part numbers.
// used for sorting the uploaded parts before completing the multipart request.
type completedParts []CompletePart

func (a completedParts) Len() int           { return len(a) }
func (a completedParts) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a completedParts) Less(i, j int) bool { return a[i].PartNumber < a[j].PartNumber }

// PutObject creates an object in a bucket.
//
// You must have WRITE permissions on a bucket to create an object.
//
//  - For size smaller than 64MiB PutObject automatically does a
//    single atomic Put operation.
//  - For size larger than 64MiB PutObject automatically does a
//    multipart Put operation.
//  - For size input as -1 PutObject does a multipart Put operation
//    until input stream reaches EOF. Maximum object size that can
//    be uploaded through this operation will be 5TiB.
func (c Client) PutObject(bucketName, objectName string, reader io.Reader, objectSize int64,
	opts PutObjectOptions) (n int64, err error) {
	return c.PutObjectWithContext(context.Background(), bucketName, objectName, reader, objectSize, opts)
}

func (c Client) putObjectCommon(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, opts PutObjectOptions) (n int64, err error) {
	// Check for largest object size allowed.
	if size > int64(maxMultipartPutObjectSize) {
		return 0, ErrEntityTooLarge(size, maxMultipartPutObjectSize, bucketName, objectName)
	}

	// NOTE: Streaming signature is not supported by GCS.
	if s3utils.IsGoogleEndpoint(c.endpointURL) {
		// Do not compute MD5 for Google Cloud Storage.
		return c.putObjectNoChecksum(ctx, bucketName, objectName, reader, size, opts)
	}

	if c.overrideSignerType.IsV2() {
		if size >= 0 && size < minPartSize {
			return c.putObjectNoChecksum(ctx, bucketName, objectName, reader, size, opts)
		}
		return c.putObjectMultipart(ctx, bucketName, objectName, reader, size, opts)
	}
	if size < 0 {
		return c.putObjectMultipartStreamNoLength(ctx, bucketName, objectName, reader, opts)
	}

	if size < minPartSize {
		return c.putObjectNoChecksum(ctx, bucketName, objectName, reader, size, opts)
	}
	// For all sizes greater than 64MiB do multipart.
	return c.putObjectMultipartStream(ctx, bucketName, objectName, reader, size, opts)
}

func (c Client) putObjectMultipartStreamNoLength(ctx context.Context, bucketName, objectName string, reader io.Reader, opts PutObjectOptions) (n int64, err error) {
	// Input validation.
	if err = s3utils.CheckValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err = s3utils.CheckValidObjectName(objectName); err != nil {
		return 0, err
	}

	// Total data read and written to server. should be equal to
	// 'size' at the end of the call.
	var totalUploadedSize int64

	// Complete multipart upload.
	var complMultipartUpload completeMultipartUpload

	// Calculate the optimal parts info for a given size.
	totalPartsCount, partSize, _, err := optimalPartInfo(-1)
	if err != nil {
		return 0, err
	}
	// Initiate a new multipart upload.
	uploadID, err := c.newUploadID(ctx, bucketName, objectName, opts)
	if err != nil {
		return 0, err
	}

	defer func() {
		if err != nil {
			c.abortMultipartUpload(ctx, bucketName, objectName, uploadID)
		}
	}()

	// Part number always starts with '1'.
	partNumber := 1

	// Initialize parts uploaded map.
	partsInfo := make(map[int]ObjectPart)

	// Create a buffer.
	buf := make([]byte, partSize)
	defer debug.FreeOSMemory()

	for partNumber <= totalPartsCount {
		length, rErr := io.ReadFull(reader, buf)
		if rErr == io.EOF && partNumber > 1 {
			break
		}
		if rErr != nil && rErr != io.ErrUnexpectedEOF {
			return 0, rErr
		}
		// Update progress reader appropriately to the latest offset
		// as we read from the source.
		rd := newHook(bytes.NewReader(buf[:length]), opts.Progress)

		// Proceed to upload the part.
		var objPart ObjectPart
		objPart, err = c.uploadPart(ctx, bucketName, objectName, uploadID, rd, partNumber,
			"", "", int64(length), opts.UserMetadata)
		if err != nil {
			return totalUploadedSize, err
		}

		// Save successfully uploaded part metadata.
		partsInfo[partNumber] = objPart

		// Save successfully uploaded size.
		totalUploadedSize += int64(length)

		// Increment part number.
		partNumber++

		// For unknown size, Read EOF we break away.
		// We do not have to upload till totalPartsCount.
		if rErr == io.EOF {
			break
		}
	}

	// Loop over total uploaded parts to save them in
	// Parts array before completing the multipart request.
	for i := 1; i < partNumber; i++ {
		part, ok := partsInfo[i]
		if !ok {
			return 0, ErrInvalidArgument(fmt.Sprintf("Missing part number %d", i))
		}
		complMultipartUpload.Parts = append(complMultipartUpload.Parts, CompletePart{
			ETag:       part.ETag,
			PartNumber: part.PartNumber,
		})
	}

	// Sort all completed parts.
	sort.Sort(completedParts(complMultipartUpload.Parts))
	if _, err = c.completeMultipartUpload(ctx, bucketName, objectName, uploadID, complMultipartUpload); err != nil {
		return totalUploadedSize, err
	}

	// Return final size.
	return totalUploadedSize, nil
}
