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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/pkg/s3utils"
)

// BucketExists verify if bucket exists and you have permission to access it.
func (c Client) BucketExists(bucketName string) (bool, error) {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return false, err
	}

	// Execute HEAD on bucketName.
	resp, err := c.executeMethod(context.Background(), "HEAD", requestMetadata{
		bucketName:       bucketName,
		contentSHA256Hex: emptySHA256Hex,
	})
	defer closeResponse(resp)
	if err != nil {
		if ToErrorResponse(err).Code == "NoSuchBucket" {
			return false, nil
		}
		return false, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return false, httpRespToErrorResponse(resp, bucketName, "")
		}
	}
	return true, nil
}

// List of header keys to be filtered, usually
// from all S3 API http responses.
var defaultFilterKeys = []string{
	"Connection",
	"Transfer-Encoding",
	"Accept-Ranges",
	"Date",
	"Server",
	"Vary",
	"x-amz-bucket-region",
	"x-amz-request-id",
	"x-amz-id-2",
	// Add new headers to be ignored.
}

// Extract only necessary metadata header key/values by
// filtering them out with a list of custom header keys.
func extractObjMetadata(header http.Header) http.Header {
	filterKeys := append([]string{
		"ETag",
		"Content-Length",
		"Last-Modified",
		"Content-Type",
	}, defaultFilterKeys...)
	return filterHeader(header, filterKeys)
}

// StatObject verifies if object exists and you have permission to access.
func (c Client) StatObject(bucketName, objectName string, opts StatObjectOptions) (ObjectInfo, error) {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return ObjectInfo{}, err
	}
	if err := s3utils.CheckValidObjectName(objectName); err != nil {
		return ObjectInfo{}, err
	}
	return c.statObject(context.Background(), bucketName, objectName, opts)
}

// Lower level API for statObject supporting pre-conditions and range headers.
func (c Client) statObject(ctx context.Context, bucketName, objectName string, opts StatObjectOptions) (ObjectInfo, error) {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return ObjectInfo{}, err
	}
	if err := s3utils.CheckValidObjectName(objectName); err != nil {
		return ObjectInfo{}, err
	}

	// Execute HEAD on objectName.
	resp, err := c.executeMethod(ctx, "HEAD", requestMetadata{
		bucketName:       bucketName,
		objectName:       objectName,
		contentSHA256Hex: emptySHA256Hex,
		customHeader:     opts.Header(),
	})
	defer closeResponse(resp)
	if err != nil {
		return ObjectInfo{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return ObjectInfo{}, httpRespToErrorResponse(resp, bucketName, objectName)
		}
	}

	// Trim off the odd double quotes from ETag in the beginning and end.
	md5sum := strings.TrimPrefix(resp.Header.Get("ETag"), "\"")
	md5sum = strings.TrimSuffix(md5sum, "\"")

	// Parse content length is exists
	var size int64 = -1
	contentLengthStr := resp.Header.Get("Content-Length")
	if contentLengthStr != "" {
		size, err = strconv.ParseInt(contentLengthStr, 10, 64)
		if err != nil {
			// Content-Length is not valid
			return ObjectInfo{}, ErrorResponse{
				Code:       "InternalError",
				Message:    "Content-Length is invalid. " + reportIssue,
				BucketName: bucketName,
				Key:        objectName,
				RequestID:  resp.Header.Get("x-amz-request-id"),
				HostID:     resp.Header.Get("x-amz-id-2"),
				Region:     resp.Header.Get("x-amz-bucket-region"),
			}
		}
	}

	// Parse Last-Modified has http time format.
	date, err := time.Parse(http.TimeFormat, resp.Header.Get("Last-Modified"))
	if err != nil {
		return ObjectInfo{}, ErrorResponse{
			Code:       "InternalError",
			Message:    "Last-Modified time format is invalid. " + reportIssue,
			BucketName: bucketName,
			Key:        objectName,
			RequestID:  resp.Header.Get("x-amz-request-id"),
			HostID:     resp.Header.Get("x-amz-id-2"),
			Region:     resp.Header.Get("x-amz-bucket-region"),
		}
	}

	// Fetch content type if any present.
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Save object metadata info.
	return ObjectInfo{
		ETag:         md5sum,
		Key:          objectName,
		Size:         size,
		LastModified: date,
		ContentType:  contentType,
		// Extract only the relevant header keys describing the object.
		// following function filters out a list of standard set of keys
		// which are not part of object metadata.
		Metadata: extractObjMetadata(resp.Header),
	}, nil
}
