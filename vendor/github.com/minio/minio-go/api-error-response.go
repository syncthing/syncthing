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
	"encoding/xml"
	"fmt"
	"net/http"
)

/* **** SAMPLE ERROR RESPONSE ****
<?xml version="1.0" encoding="UTF-8"?>
<Error>
   <Code>AccessDenied</Code>
   <Message>Access Denied</Message>
   <BucketName>bucketName</BucketName>
   <Key>objectName</Key>
   <RequestId>F19772218238A85A</RequestId>
   <HostId>GuWkjyviSiGHizehqpmsD1ndz5NClSP19DOT+s2mv7gXGQ8/X1lhbDGiIJEXpGFD</HostId>
</Error>
*/

// ErrorResponse - Is the typed error returned by all API operations.
type ErrorResponse struct {
	XMLName    xml.Name `xml:"Error" json:"-"`
	Code       string
	Message    string
	BucketName string
	Key        string
	RequestID  string `xml:"RequestId"`
	HostID     string `xml:"HostId"`

	// Region where the bucket is located. This header is returned
	// only in HEAD bucket and ListObjects response.
	Region string

	// Underlying HTTP status code for the returned error
	StatusCode int `xml:"-" json:"-"`

	// Headers of the returned S3 XML error
	Headers http.Header `xml:"-" json:"-"`
}

// ToErrorResponse - Returns parsed ErrorResponse struct from body and
// http headers.
//
// For example:
//
//   import s3 "github.com/minio/minio-go"
//   ...
//   ...
//   reader, stat, err := s3.GetObject(...)
//   if err != nil {
//      resp := s3.ToErrorResponse(err)
//   }
//   ...
func ToErrorResponse(err error) ErrorResponse {
	switch err := err.(type) {
	case ErrorResponse:
		return err
	default:
		return ErrorResponse{}
	}
}

// Error - Returns S3 error string.
func (e ErrorResponse) Error() string {
	if e.Message == "" {
		msg, ok := s3ErrorResponseMap[e.Code]
		if !ok {
			msg = fmt.Sprintf("Error response code %s.", e.Code)
		}
		return msg
	}
	return e.Message
}

// Common string for errors to report issue location in unexpected
// cases.
const (
	reportIssue = "Please report this issue at https://github.com/minio/minio-go/issues."
)

// httpRespToErrorResponse returns a new encoded ErrorResponse
// structure as error.
func httpRespToErrorResponse(resp *http.Response, bucketName, objectName string) error {
	if resp == nil {
		msg := "Response is empty. " + reportIssue
		return ErrInvalidArgument(msg)
	}

	errResp := ErrorResponse{
		StatusCode: resp.StatusCode,
	}

	err := xmlDecoder(resp.Body, &errResp)
	// Xml decoding failed with no body, fall back to HTTP headers.
	if err != nil {
		switch resp.StatusCode {
		case http.StatusNotFound:
			if objectName == "" {
				errResp = ErrorResponse{
					StatusCode: resp.StatusCode,
					Code:       "NoSuchBucket",
					Message:    "The specified bucket does not exist.",
					BucketName: bucketName,
				}
			} else {
				errResp = ErrorResponse{
					StatusCode: resp.StatusCode,
					Code:       "NoSuchKey",
					Message:    "The specified key does not exist.",
					BucketName: bucketName,
					Key:        objectName,
				}
			}
		case http.StatusForbidden:
			errResp = ErrorResponse{
				StatusCode: resp.StatusCode,
				Code:       "AccessDenied",
				Message:    "Access Denied.",
				BucketName: bucketName,
				Key:        objectName,
			}
		case http.StatusConflict:
			errResp = ErrorResponse{
				StatusCode: resp.StatusCode,
				Code:       "Conflict",
				Message:    "Bucket not empty.",
				BucketName: bucketName,
			}
		case http.StatusPreconditionFailed:
			errResp = ErrorResponse{
				StatusCode: resp.StatusCode,
				Code:       "PreconditionFailed",
				Message:    s3ErrorResponseMap["PreconditionFailed"],
				BucketName: bucketName,
				Key:        objectName,
			}
		default:
			errResp = ErrorResponse{
				StatusCode: resp.StatusCode,
				Code:       resp.Status,
				Message:    resp.Status,
				BucketName: bucketName,
			}
		}
	}

	// Save hostID, requestID and region information
	// from headers if not available through error XML.
	if errResp.RequestID == "" {
		errResp.RequestID = resp.Header.Get("x-amz-request-id")
	}
	if errResp.HostID == "" {
		errResp.HostID = resp.Header.Get("x-amz-id-2")
	}
	if errResp.Region == "" {
		errResp.Region = resp.Header.Get("x-amz-bucket-region")
	}
	if errResp.Code == "InvalidRegion" && errResp.Region != "" {
		errResp.Message = fmt.Sprintf("Region does not match, expecting region ‘%s’.", errResp.Region)
	}

	// Save headers returned in the API XML error
	errResp.Headers = resp.Header

	return errResp
}

// ErrTransferAccelerationBucket - bucket name is invalid to be used with transfer acceleration.
func ErrTransferAccelerationBucket(bucketName string) error {
	return ErrorResponse{
		StatusCode: http.StatusBadRequest,
		Code:       "InvalidArgument",
		Message:    "The name of the bucket used for Transfer Acceleration must be DNS-compliant and must not contain periods ‘.’.",
		BucketName: bucketName,
	}
}

// ErrEntityTooLarge - Input size is larger than supported maximum.
func ErrEntityTooLarge(totalSize, maxObjectSize int64, bucketName, objectName string) error {
	msg := fmt.Sprintf("Your proposed upload size ‘%d’ exceeds the maximum allowed object size ‘%d’ for single PUT operation.", totalSize, maxObjectSize)
	return ErrorResponse{
		StatusCode: http.StatusBadRequest,
		Code:       "EntityTooLarge",
		Message:    msg,
		BucketName: bucketName,
		Key:        objectName,
	}
}

// ErrEntityTooSmall - Input size is smaller than supported minimum.
func ErrEntityTooSmall(totalSize int64, bucketName, objectName string) error {
	msg := fmt.Sprintf("Your proposed upload size ‘%d’ is below the minimum allowed object size ‘0B’ for single PUT operation.", totalSize)
	return ErrorResponse{
		StatusCode: http.StatusBadRequest,
		Code:       "EntityTooSmall",
		Message:    msg,
		BucketName: bucketName,
		Key:        objectName,
	}
}

// ErrUnexpectedEOF - Unexpected end of file reached.
func ErrUnexpectedEOF(totalRead, totalSize int64, bucketName, objectName string) error {
	msg := fmt.Sprintf("Data read ‘%d’ is not equal to the size ‘%d’ of the input Reader.", totalRead, totalSize)
	return ErrorResponse{
		StatusCode: http.StatusBadRequest,
		Code:       "UnexpectedEOF",
		Message:    msg,
		BucketName: bucketName,
		Key:        objectName,
	}
}

// ErrInvalidBucketName - Invalid bucket name response.
func ErrInvalidBucketName(message string) error {
	return ErrorResponse{
		StatusCode: http.StatusBadRequest,
		Code:       "InvalidBucketName",
		Message:    message,
		RequestID:  "minio",
	}
}

// ErrInvalidObjectName - Invalid object name response.
func ErrInvalidObjectName(message string) error {
	return ErrorResponse{
		StatusCode: http.StatusNotFound,
		Code:       "NoSuchKey",
		Message:    message,
		RequestID:  "minio",
	}
}

// ErrInvalidObjectPrefix - Invalid object prefix response is
// similar to object name response.
var ErrInvalidObjectPrefix = ErrInvalidObjectName

// ErrInvalidArgument - Invalid argument response.
func ErrInvalidArgument(message string) error {
	return ErrorResponse{
		StatusCode: http.StatusBadRequest,
		Code:       "InvalidArgument",
		Message:    message,
		RequestID:  "minio",
	}
}

// ErrNoSuchBucketPolicy - No Such Bucket Policy response
// The specified bucket does not have a bucket policy.
func ErrNoSuchBucketPolicy(message string) error {
	return ErrorResponse{
		StatusCode: http.StatusNotFound,
		Code:       "NoSuchBucketPolicy",
		Message:    message,
		RequestID:  "minio",
	}
}

// ErrAPINotSupported - API not supported response
// The specified API call is not supported
func ErrAPINotSupported(message string) error {
	return ErrorResponse{
		StatusCode: http.StatusNotImplemented,
		Code:       "APINotSupported",
		Message:    message,
		RequestID:  "minio",
	}
}
