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
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"

	"github.com/minio/minio-go/pkg/policy"
	"github.com/minio/minio-go/pkg/s3utils"
)

/// Bucket operations

// MakeBucket creates a new bucket with bucketName.
//
// Location is an optional argument, by default all buckets are
// created in US Standard Region.
//
// For Amazon S3 for more supported regions - http://docs.aws.amazon.com/general/latest/gr/rande.html
// For Google Cloud Storage for more supported regions - https://cloud.google.com/storage/docs/bucket-locations
func (c Client) MakeBucket(bucketName string, location string) (err error) {
	defer func() {
		// Save the location into cache on a successful makeBucket response.
		if err == nil {
			c.bucketLocCache.Set(bucketName, location)
		}
	}()

	// Validate the input arguments.
	if err := s3utils.CheckValidBucketNameStrict(bucketName); err != nil {
		return err
	}

	// If location is empty, treat is a default region 'us-east-1'.
	if location == "" {
		location = "us-east-1"
		// For custom region clients, default
		// to custom region instead not 'us-east-1'.
		if c.region != "" {
			location = c.region
		}
	}
	// PUT bucket request metadata.
	reqMetadata := requestMetadata{
		bucketName:     bucketName,
		bucketLocation: location,
	}

	// If location is not 'us-east-1' create bucket location config.
	if location != "us-east-1" && location != "" {
		createBucketConfig := createBucketConfiguration{}
		createBucketConfig.Location = location
		var createBucketConfigBytes []byte
		createBucketConfigBytes, err = xml.Marshal(createBucketConfig)
		if err != nil {
			return err
		}
		reqMetadata.contentMD5Base64 = sumMD5Base64(createBucketConfigBytes)
		reqMetadata.contentSHA256Hex = sum256Hex(createBucketConfigBytes)
		reqMetadata.contentBody = bytes.NewReader(createBucketConfigBytes)
		reqMetadata.contentLength = int64(len(createBucketConfigBytes))
	}

	// Execute PUT to create a new bucket.
	resp, err := c.executeMethod(context.Background(), "PUT", reqMetadata)
	defer closeResponse(resp)
	if err != nil {
		return err
	}

	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return httpRespToErrorResponse(resp, bucketName, "")
		}
	}

	// Success.
	return nil
}

// SetBucketPolicy set the access permissions on an existing bucket.
//
// For example
//
//  none - owner gets full access [default].
//  readonly - anonymous get access for everyone at a given object prefix.
//  readwrite - anonymous list/put/delete access to a given object prefix.
//  writeonly - anonymous put/delete access to a given object prefix.
func (c Client) SetBucketPolicy(bucketName string, objectPrefix string, bucketPolicy policy.BucketPolicy) error {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return err
	}
	if err := s3utils.CheckValidObjectNamePrefix(objectPrefix); err != nil {
		return err
	}

	if !bucketPolicy.IsValidBucketPolicy() {
		return ErrInvalidArgument(fmt.Sprintf("Invalid bucket policy provided. %s", bucketPolicy))
	}

	policyInfo, err := c.getBucketPolicy(bucketName)
	errResponse := ToErrorResponse(err)
	if err != nil && errResponse.Code != "NoSuchBucketPolicy" {
		return err
	}

	if bucketPolicy == policy.BucketPolicyNone && policyInfo.Statements == nil {
		// As the request is for removing policy and the bucket
		// has empty policy statements, just return success.
		return nil
	}

	policyInfo.Statements = policy.SetPolicy(policyInfo.Statements, bucketPolicy, bucketName, objectPrefix)

	// Save the updated policies.
	return c.putBucketPolicy(bucketName, policyInfo)
}

// Saves a new bucket policy.
func (c Client) putBucketPolicy(bucketName string, policyInfo policy.BucketAccessPolicy) error {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return err
	}

	// If there are no policy statements, we should remove entire policy.
	if len(policyInfo.Statements) == 0 {
		return c.removeBucketPolicy(bucketName)
	}

	// Get resources properly escaped and lined up before
	// using them in http request.
	urlValues := make(url.Values)
	urlValues.Set("policy", "")

	policyBytes, err := json.Marshal(&policyInfo)
	if err != nil {
		return err
	}

	policyBuffer := bytes.NewReader(policyBytes)
	reqMetadata := requestMetadata{
		bucketName:       bucketName,
		queryValues:      urlValues,
		contentBody:      policyBuffer,
		contentLength:    int64(len(policyBytes)),
		contentMD5Base64: sumMD5Base64(policyBytes),
		contentSHA256Hex: sum256Hex(policyBytes),
	}

	// Execute PUT to upload a new bucket policy.
	resp, err := c.executeMethod(context.Background(), "PUT", reqMetadata)
	defer closeResponse(resp)
	if err != nil {
		return err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusNoContent {
			return httpRespToErrorResponse(resp, bucketName, "")
		}
	}
	return nil
}

// Removes all policies on a bucket.
func (c Client) removeBucketPolicy(bucketName string) error {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return err
	}
	// Get resources properly escaped and lined up before
	// using them in http request.
	urlValues := make(url.Values)
	urlValues.Set("policy", "")

	// Execute DELETE on objectName.
	resp, err := c.executeMethod(context.Background(), "DELETE", requestMetadata{
		bucketName:       bucketName,
		queryValues:      urlValues,
		contentSHA256Hex: emptySHA256Hex,
	})
	defer closeResponse(resp)
	if err != nil {
		return err
	}
	return nil
}

// SetBucketNotification saves a new bucket notification.
func (c Client) SetBucketNotification(bucketName string, bucketNotification BucketNotification) error {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return err
	}

	// Get resources properly escaped and lined up before
	// using them in http request.
	urlValues := make(url.Values)
	urlValues.Set("notification", "")

	notifBytes, err := xml.Marshal(bucketNotification)
	if err != nil {
		return err
	}

	notifBuffer := bytes.NewReader(notifBytes)
	reqMetadata := requestMetadata{
		bucketName:       bucketName,
		queryValues:      urlValues,
		contentBody:      notifBuffer,
		contentLength:    int64(len(notifBytes)),
		contentMD5Base64: sumMD5Base64(notifBytes),
		contentSHA256Hex: sum256Hex(notifBytes),
	}

	// Execute PUT to upload a new bucket notification.
	resp, err := c.executeMethod(context.Background(), "PUT", reqMetadata)
	defer closeResponse(resp)
	if err != nil {
		return err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return httpRespToErrorResponse(resp, bucketName, "")
		}
	}
	return nil
}

// RemoveAllBucketNotification - Remove bucket notification clears all previously specified config
func (c Client) RemoveAllBucketNotification(bucketName string) error {
	return c.SetBucketNotification(bucketName, BucketNotification{})
}
