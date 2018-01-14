// +build ignore

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

package main

import (
	"log"

	"github.com/minio/minio-go"
)

func main() {
	// Note: YOUR-ACCESSKEYID, YOUR-SECRETACCESSKEY and my-bucketname are
	// dummy values, please replace them with original values.

	// Requests are always secure (HTTPS) by default. Set secure=false to enable insecure (HTTP) access.
	// This boolean value is the last argument for New().

	// New returns an Amazon S3 compatible client object. API compatibility (v2 or v4) is automatically
	// determined based on the Endpoint value.
	s3Client, err := minio.New("s3.amazonaws.com", "YOUR-ACCESSKEYID", "YOUR-SECRETACCESSKEY", true)
	if err != nil {
		log.Fatalln(err)
	}

	// s3Client.TraceOn(os.Stderr)

	// Fetch the policy at 'my-objectprefix'.
	policy, err := s3Client.GetBucketPolicy("my-bucketname", "my-objectprefix")
	if err != nil {
		log.Fatalln(err)
	}

	// Description of policy output.
	// "none" -  The specified bucket does not have a bucket policy.
	// "readonly" - Read only operations are allowed.
	// "writeonly" - Write only operations are allowed.
	// "readwrite" - both read and write operations are allowed, the bucket is public.
	log.Println("Success - ", policy)
}
