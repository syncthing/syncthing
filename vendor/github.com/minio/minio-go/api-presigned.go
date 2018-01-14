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
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/minio/minio-go/pkg/s3signer"
	"github.com/minio/minio-go/pkg/s3utils"
)

// presignURL - Returns a presigned URL for an input 'method'.
// Expires maximum is 7days - ie. 604800 and minimum is 1.
func (c Client) presignURL(method string, bucketName string, objectName string, expires time.Duration, reqParams url.Values) (u *url.URL, err error) {
	// Input validation.
	if method == "" {
		return nil, ErrInvalidArgument("method cannot be empty.")
	}
	if err = s3utils.CheckValidBucketName(bucketName); err != nil {
		return nil, err
	}
	if err = isValidExpiry(expires); err != nil {
		return nil, err
	}

	// Convert expires into seconds.
	expireSeconds := int64(expires / time.Second)
	reqMetadata := requestMetadata{
		presignURL:  true,
		bucketName:  bucketName,
		objectName:  objectName,
		expires:     expireSeconds,
		queryValues: reqParams,
	}

	// Instantiate a new request.
	// Since expires is set newRequest will presign the request.
	var req *http.Request
	if req, err = c.newRequest(method, reqMetadata); err != nil {
		return nil, err
	}
	return req.URL, nil
}

// PresignedGetObject - Returns a presigned URL to access an object
// data without credentials. URL can have a maximum expiry of
// upto 7days or a minimum of 1sec. Additionally you can override
// a set of response headers using the query parameters.
func (c Client) PresignedGetObject(bucketName string, objectName string, expires time.Duration, reqParams url.Values) (u *url.URL, err error) {
	if err = s3utils.CheckValidObjectName(objectName); err != nil {
		return nil, err
	}
	return c.presignURL("GET", bucketName, objectName, expires, reqParams)
}

// PresignedHeadObject - Returns a presigned URL to access object
// metadata without credentials. URL can have a maximum expiry of
// upto 7days or a minimum of 1sec. Additionally you can override
// a set of response headers using the query parameters.
func (c Client) PresignedHeadObject(bucketName string, objectName string, expires time.Duration, reqParams url.Values) (u *url.URL, err error) {
	if err = s3utils.CheckValidObjectName(objectName); err != nil {
		return nil, err
	}
	return c.presignURL("HEAD", bucketName, objectName, expires, reqParams)
}

// PresignedPutObject - Returns a presigned URL to upload an object
// without credentials. URL can have a maximum expiry of upto 7days
// or a minimum of 1sec.
func (c Client) PresignedPutObject(bucketName string, objectName string, expires time.Duration) (u *url.URL, err error) {
	if err = s3utils.CheckValidObjectName(objectName); err != nil {
		return nil, err
	}
	return c.presignURL("PUT", bucketName, objectName, expires, nil)
}

// Presign - returns a presigned URL for any http method of your choice
// along with custom request params. URL can have a maximum expiry of
// upto 7days or a minimum of 1sec.
func (c Client) Presign(method string, bucketName string, objectName string, expires time.Duration, reqParams url.Values) (u *url.URL, err error) {
	return c.presignURL(method, bucketName, objectName, expires, reqParams)
}

// PresignedPostPolicy - Returns POST urlString, form data to upload an object.
func (c Client) PresignedPostPolicy(p *PostPolicy) (u *url.URL, formData map[string]string, err error) {
	// Validate input arguments.
	if p.expiration.IsZero() {
		return nil, nil, errors.New("Expiration time must be specified")
	}
	if _, ok := p.formData["key"]; !ok {
		return nil, nil, errors.New("object key must be specified")
	}
	if _, ok := p.formData["bucket"]; !ok {
		return nil, nil, errors.New("bucket name must be specified")
	}

	bucketName := p.formData["bucket"]
	// Fetch the bucket location.
	location, err := c.getBucketLocation(bucketName)
	if err != nil {
		return nil, nil, err
	}

	u, err = c.makeTargetURL(bucketName, "", location, nil)
	if err != nil {
		return nil, nil, err
	}

	// Get credentials from the configured credentials provider.
	credValues, err := c.credsProvider.Get()
	if err != nil {
		return nil, nil, err
	}

	var (
		signerType      = credValues.SignerType
		sessionToken    = credValues.SessionToken
		accessKeyID     = credValues.AccessKeyID
		secretAccessKey = credValues.SecretAccessKey
	)

	if signerType.IsAnonymous() {
		return nil, nil, ErrInvalidArgument("Presigned operations are not supported for anonymous credentials")
	}

	// Keep time.
	t := time.Now().UTC()
	// For signature version '2' handle here.
	if signerType.IsV2() {
		policyBase64 := p.base64()
		p.formData["policy"] = policyBase64
		// For Google endpoint set this value to be 'GoogleAccessId'.
		if s3utils.IsGoogleEndpoint(c.endpointURL) {
			p.formData["GoogleAccessId"] = accessKeyID
		} else {
			// For all other endpoints set this value to be 'AWSAccessKeyId'.
			p.formData["AWSAccessKeyId"] = accessKeyID
		}
		// Sign the policy.
		p.formData["signature"] = s3signer.PostPresignSignatureV2(policyBase64, secretAccessKey)
		return u, p.formData, nil
	}

	// Add date policy.
	if err = p.addNewPolicy(policyCondition{
		matchType: "eq",
		condition: "$x-amz-date",
		value:     t.Format(iso8601DateFormat),
	}); err != nil {
		return nil, nil, err
	}

	// Add algorithm policy.
	if err = p.addNewPolicy(policyCondition{
		matchType: "eq",
		condition: "$x-amz-algorithm",
		value:     signV4Algorithm,
	}); err != nil {
		return nil, nil, err
	}

	// Add a credential policy.
	credential := s3signer.GetCredential(accessKeyID, location, t)
	if err = p.addNewPolicy(policyCondition{
		matchType: "eq",
		condition: "$x-amz-credential",
		value:     credential,
	}); err != nil {
		return nil, nil, err
	}

	if sessionToken != "" {
		if err = p.addNewPolicy(policyCondition{
			matchType: "eq",
			condition: "$x-amz-security-token",
			value:     sessionToken,
		}); err != nil {
			return nil, nil, err
		}
	}

	// Get base64 encoded policy.
	policyBase64 := p.base64()

	// Fill in the form data.
	p.formData["policy"] = policyBase64
	p.formData["x-amz-algorithm"] = signV4Algorithm
	p.formData["x-amz-credential"] = credential
	p.formData["x-amz-date"] = t.Format(iso8601DateFormat)
	if sessionToken != "" {
		p.formData["x-amz-security-token"] = sessionToken
	}
	p.formData["x-amz-signature"] = s3signer.PostPresignSignatureV4(policyBase64, t, secretAccessKey, location)
	return u, p.formData, nil
}
