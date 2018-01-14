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
	"context"
	"io"
	"strings"

	"github.com/minio/minio-go/pkg/policy"
)

// Core - Inherits Client and adds new methods to expose the low level S3 APIs.
type Core struct {
	*Client
}

// NewCore - Returns new initialized a Core client, this CoreClient should be
// only used under special conditions such as need to access lower primitives
// and being able to use them to write your own wrappers.
func NewCore(endpoint string, accessKeyID, secretAccessKey string, secure bool) (*Core, error) {
	var s3Client Core
	client, err := NewV4(endpoint, accessKeyID, secretAccessKey, secure)
	if err != nil {
		return nil, err
	}
	s3Client.Client = client
	return &s3Client, nil
}

// ListObjects - List all the objects at a prefix, optionally with marker and delimiter
// you can further filter the results.
func (c Core) ListObjects(bucket, prefix, marker, delimiter string, maxKeys int) (result ListBucketResult, err error) {
	return c.listObjectsQuery(bucket, prefix, marker, delimiter, maxKeys)
}

// ListObjectsV2 - Lists all the objects at a prefix, similar to ListObjects() but uses
// continuationToken instead of marker to further filter the results.
func (c Core) ListObjectsV2(bucketName, objectPrefix, continuationToken string, fetchOwner bool, delimiter string, maxkeys int) (ListBucketV2Result, error) {
	return c.listObjectsV2Query(bucketName, objectPrefix, continuationToken, fetchOwner, delimiter, maxkeys)
}

// CopyObject - copies an object from source object to destination object on server side.
func (c Core) CopyObject(sourceBucket, sourceObject, destBucket, destObject string, metadata map[string]string) (ObjectInfo, error) {
	return c.copyObjectDo(context.Background(), sourceBucket, sourceObject, destBucket, destObject, metadata)
}

// CopyObjectPart - creates a part in a multipart upload by copying (a
// part of) an existing object.
func (c Core) CopyObjectPart(srcBucket, srcObject, destBucket, destObject string, uploadID string,
	partID int, startOffset, length int64, metadata map[string]string) (p CompletePart, err error) {

	return c.copyObjectPartDo(context.Background(), srcBucket, srcObject, destBucket, destObject, uploadID,
		partID, startOffset, length, metadata)
}

// PutObject - Upload object. Uploads using single PUT call.
func (c Core) PutObject(bucket, object string, data io.Reader, size int64, md5Base64, sha256Hex string, metadata map[string]string) (ObjectInfo, error) {
	opts := PutObjectOptions{}
	m := make(map[string]string)
	for k, v := range metadata {
		if strings.ToLower(k) == "content-encoding" {
			opts.ContentEncoding = v
		} else if strings.ToLower(k) == "content-disposition" {
			opts.ContentDisposition = v
		} else if strings.ToLower(k) == "content-type" {
			opts.ContentType = v
		} else if strings.ToLower(k) == "cache-control" {
			opts.CacheControl = v
		} else {
			m[k] = metadata[k]
		}
	}
	opts.UserMetadata = m
	return c.putObjectDo(context.Background(), bucket, object, data, md5Base64, sha256Hex, size, opts)
}

// NewMultipartUpload - Initiates new multipart upload and returns the new uploadID.
func (c Core) NewMultipartUpload(bucket, object string, opts PutObjectOptions) (uploadID string, err error) {
	result, err := c.initiateMultipartUpload(context.Background(), bucket, object, opts)
	return result.UploadID, err
}

// ListMultipartUploads - List incomplete uploads.
func (c Core) ListMultipartUploads(bucket, prefix, keyMarker, uploadIDMarker, delimiter string, maxUploads int) (result ListMultipartUploadsResult, err error) {
	return c.listMultipartUploadsQuery(bucket, keyMarker, uploadIDMarker, prefix, delimiter, maxUploads)
}

// PutObjectPart - Upload an object part.
func (c Core) PutObjectPart(bucket, object, uploadID string, partID int, data io.Reader, size int64, md5Base64, sha256Hex string) (ObjectPart, error) {
	return c.PutObjectPartWithMetadata(bucket, object, uploadID, partID, data, size, md5Base64, sha256Hex, nil)
}

// PutObjectPartWithMetadata - upload an object part with additional request metadata.
func (c Core) PutObjectPartWithMetadata(bucket, object, uploadID string, partID int, data io.Reader,
	size int64, md5Base64, sha256Hex string, metadata map[string]string) (ObjectPart, error) {
	return c.uploadPart(context.Background(), bucket, object, uploadID, data, partID, md5Base64, sha256Hex, size, metadata)
}

// ListObjectParts - List uploaded parts of an incomplete upload.x
func (c Core) ListObjectParts(bucket, object, uploadID string, partNumberMarker int, maxParts int) (result ListObjectPartsResult, err error) {
	return c.listObjectPartsQuery(bucket, object, uploadID, partNumberMarker, maxParts)
}

// CompleteMultipartUpload - Concatenate uploaded parts and commit to an object.
func (c Core) CompleteMultipartUpload(bucket, object, uploadID string, parts []CompletePart) error {
	_, err := c.completeMultipartUpload(context.Background(), bucket, object, uploadID, completeMultipartUpload{
		Parts: parts,
	})
	return err
}

// AbortMultipartUpload - Abort an incomplete upload.
func (c Core) AbortMultipartUpload(bucket, object, uploadID string) error {
	return c.abortMultipartUpload(context.Background(), bucket, object, uploadID)
}

// GetBucketPolicy - fetches bucket access policy for a given bucket.
func (c Core) GetBucketPolicy(bucket string) (policy.BucketAccessPolicy, error) {
	return c.getBucketPolicy(bucket)
}

// PutBucketPolicy - applies a new bucket access policy for a given bucket.
func (c Core) PutBucketPolicy(bucket string, bucketPolicy policy.BucketAccessPolicy) error {
	return c.putBucketPolicy(bucket, bucketPolicy)
}

// GetObject is a lower level API implemented to support reading
// partial objects and also downloading objects with special conditions
// matching etag, modtime etc.
func (c Core) GetObject(bucketName, objectName string, opts GetObjectOptions) (io.ReadCloser, ObjectInfo, error) {
	return c.getObject(context.Background(), bucketName, objectName, opts)
}

// StatObject is a lower level API implemented to support special
// conditions matching etag, modtime on a request.
func (c Core) StatObject(bucketName, objectName string, opts StatObjectOptions) (ObjectInfo, error) {
	return c.statObject(context.Background(), bucketName, objectName, opts)
}
