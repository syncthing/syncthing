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

// awsS3EndpointMap Amazon S3 endpoint map.
// "cn-north-1" adds support for AWS China.
var awsS3EndpointMap = map[string]string{
	"us-east-1":      "s3.amazonaws.com",
	"us-east-2":      "s3-us-east-2.amazonaws.com",
	"us-west-2":      "s3-us-west-2.amazonaws.com",
	"us-west-1":      "s3-us-west-1.amazonaws.com",
	"ca-central-1":   "s3.ca-central-1.amazonaws.com",
	"eu-west-1":      "s3-eu-west-1.amazonaws.com",
	"eu-west-2":      "s3-eu-west-2.amazonaws.com",
	"eu-central-1":   "s3-eu-central-1.amazonaws.com",
	"ap-south-1":     "s3-ap-south-1.amazonaws.com",
	"ap-southeast-1": "s3-ap-southeast-1.amazonaws.com",
	"ap-southeast-2": "s3-ap-southeast-2.amazonaws.com",
	"ap-northeast-1": "s3-ap-northeast-1.amazonaws.com",
	"ap-northeast-2": "s3-ap-northeast-2.amazonaws.com",
	"sa-east-1":      "s3-sa-east-1.amazonaws.com",
	"us-gov-west-1":  "s3-us-gov-west-1.amazonaws.com",
	"cn-north-1":     "s3.cn-north-1.amazonaws.com.cn",
}

// getS3Endpoint get Amazon S3 endpoint based on the bucket location.
func getS3Endpoint(bucketLocation string) (s3Endpoint string) {
	s3Endpoint, ok := awsS3EndpointMap[bucketLocation]
	if !ok {
		// Default to 's3.amazonaws.com' endpoint.
		s3Endpoint = "s3.amazonaws.com"
	}
	return s3Endpoint
}
