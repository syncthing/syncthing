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
	"encoding/xml"
	"io"
	"net/http"
	"net/url"

	"github.com/minio/minio-go/pkg/s3utils"
)

// RemoveBucket deletes the bucket name.
//
//  All objects (including all object versions and delete markers).
//  in the bucket must be deleted before successfully attempting this request.
func (c Client) RemoveBucket(bucketName string) error {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return err
	}
	// Execute DELETE on bucket.
	resp, err := c.executeMethod(context.Background(), "DELETE", requestMetadata{
		bucketName:       bucketName,
		contentSHA256Hex: emptySHA256Hex,
	})
	defer closeResponse(resp)
	if err != nil {
		return err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusNoContent {
			return httpRespToErrorResponse(resp, bucketName, "")
		}
	}

	// Remove the location from cache on a successful delete.
	c.bucketLocCache.Delete(bucketName)

	return nil
}

// RemoveObject remove an object from a bucket.
func (c Client) RemoveObject(bucketName, objectName string) error {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return err
	}
	if err := s3utils.CheckValidObjectName(objectName); err != nil {
		return err
	}
	// Execute DELETE on objectName.
	resp, err := c.executeMethod(context.Background(), "DELETE", requestMetadata{
		bucketName:       bucketName,
		objectName:       objectName,
		contentSHA256Hex: emptySHA256Hex,
	})
	defer closeResponse(resp)
	if err != nil {
		return err
	}
	if resp != nil {
		// if some unexpected error happened and max retry is reached, we want to let client know
		if resp.StatusCode != http.StatusNoContent {
			return httpRespToErrorResponse(resp, bucketName, objectName)
		}
	}

	// DeleteObject always responds with http '204' even for
	// objects which do not exist. So no need to handle them
	// specifically.
	return nil
}

// RemoveObjectError - container of Multi Delete S3 API error
type RemoveObjectError struct {
	ObjectName string
	Err        error
}

// generateRemoveMultiObjects - generate the XML request for remove multi objects request
func generateRemoveMultiObjectsRequest(objects []string) []byte {
	rmObjects := []deleteObject{}
	for _, obj := range objects {
		rmObjects = append(rmObjects, deleteObject{Key: obj})
	}
	xmlBytes, _ := xml.Marshal(deleteMultiObjects{Objects: rmObjects, Quiet: true})
	return xmlBytes
}

// processRemoveMultiObjectsResponse - parse the remove multi objects web service
// and return the success/failure result status for each object
func processRemoveMultiObjectsResponse(body io.Reader, objects []string, errorCh chan<- RemoveObjectError) {
	// Parse multi delete XML response
	rmResult := &deleteMultiObjectsResult{}
	err := xmlDecoder(body, rmResult)
	if err != nil {
		errorCh <- RemoveObjectError{ObjectName: "", Err: err}
		return
	}

	// Fill deletion that returned an error.
	for _, obj := range rmResult.UnDeletedObjects {
		errorCh <- RemoveObjectError{
			ObjectName: obj.Key,
			Err: ErrorResponse{
				Code:    obj.Code,
				Message: obj.Message,
			},
		}
	}
}

// RemoveObjects remove multiples objects from a bucket.
// The list of objects to remove are received from objectsCh.
// Remove failures are sent back via error channel.
func (c Client) RemoveObjects(bucketName string, objectsCh <-chan string) <-chan RemoveObjectError {
	errorCh := make(chan RemoveObjectError, 1)

	// Validate if bucket name is valid.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		defer close(errorCh)
		errorCh <- RemoveObjectError{
			Err: err,
		}
		return errorCh
	}
	// Validate objects channel to be properly allocated.
	if objectsCh == nil {
		defer close(errorCh)
		errorCh <- RemoveObjectError{
			Err: ErrInvalidArgument("Objects channel cannot be nil"),
		}
		return errorCh
	}

	// Generate and call MultiDelete S3 requests based on entries received from objectsCh
	go func(errorCh chan<- RemoveObjectError) {
		maxEntries := 1000
		finish := false
		urlValues := make(url.Values)
		urlValues.Set("delete", "")

		// Close error channel when Multi delete finishes.
		defer close(errorCh)

		// Loop over entries by 1000 and call MultiDelete requests
		for {
			if finish {
				break
			}
			count := 0
			var batch []string

			// Try to gather 1000 entries
			for object := range objectsCh {
				batch = append(batch, object)
				if count++; count >= maxEntries {
					break
				}
			}
			if count == 0 {
				// Multi Objects Delete API doesn't accept empty object list, quit immediately
				break
			}
			if count < maxEntries {
				// We didn't have 1000 entries, so this is the last batch
				finish = true
			}

			// Generate remove multi objects XML request
			removeBytes := generateRemoveMultiObjectsRequest(batch)
			// Execute GET on bucket to list objects.
			resp, err := c.executeMethod(context.Background(), "POST", requestMetadata{
				bucketName:       bucketName,
				queryValues:      urlValues,
				contentBody:      bytes.NewReader(removeBytes),
				contentLength:    int64(len(removeBytes)),
				contentMD5Base64: sumMD5Base64(removeBytes),
				contentSHA256Hex: sum256Hex(removeBytes),
			})
			if err != nil {
				for _, b := range batch {
					errorCh <- RemoveObjectError{ObjectName: b, Err: err}
				}
				continue
			}

			// Process multiobjects remove xml response
			processRemoveMultiObjectsResponse(resp.Body, batch, errorCh)

			closeResponse(resp)
		}
	}(errorCh)
	return errorCh
}

// RemoveIncompleteUpload aborts an partially uploaded object.
func (c Client) RemoveIncompleteUpload(bucketName, objectName string) error {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return err
	}
	if err := s3utils.CheckValidObjectName(objectName); err != nil {
		return err
	}
	// Find multipart upload id of the object to be aborted.
	uploadID, err := c.findUploadID(bucketName, objectName)
	if err != nil {
		return err
	}
	if uploadID != "" {
		// Upload id found, abort the incomplete multipart upload.
		err := c.abortMultipartUpload(context.Background(), bucketName, objectName, uploadID)
		if err != nil {
			return err
		}
	}
	return nil
}

// abortMultipartUpload aborts a multipart upload for the given
// uploadID, all previously uploaded parts are deleted.
func (c Client) abortMultipartUpload(ctx context.Context, bucketName, objectName, uploadID string) error {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return err
	}
	if err := s3utils.CheckValidObjectName(objectName); err != nil {
		return err
	}

	// Initialize url queries.
	urlValues := make(url.Values)
	urlValues.Set("uploadId", uploadID)

	// Execute DELETE on multipart upload.
	resp, err := c.executeMethod(ctx, "DELETE", requestMetadata{
		bucketName:       bucketName,
		objectName:       objectName,
		queryValues:      urlValues,
		contentSHA256Hex: emptySHA256Hex,
	})
	defer closeResponse(resp)
	if err != nil {
		return err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusNoContent {
			// Abort has no response body, handle it for any errors.
			var errorResponse ErrorResponse
			switch resp.StatusCode {
			case http.StatusNotFound:
				// This is needed specifically for abort and it cannot
				// be converged into default case.
				errorResponse = ErrorResponse{
					Code:       "NoSuchUpload",
					Message:    "The specified multipart upload does not exist.",
					BucketName: bucketName,
					Key:        objectName,
					RequestID:  resp.Header.Get("x-amz-request-id"),
					HostID:     resp.Header.Get("x-amz-id-2"),
					Region:     resp.Header.Get("x-amz-bucket-region"),
				}
			default:
				return httpRespToErrorResponse(resp, bucketName, objectName)
			}
			return errorResponse
		}
	}
	return nil
}
