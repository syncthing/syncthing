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
	"mime"
	"os"
	"path/filepath"

	"github.com/minio/minio-go/pkg/s3utils"
)

// FPutObjectWithContext - Create an object in a bucket, with contents from file at filePath. Allows request cancellation.
func (c Client) FPutObjectWithContext(ctx context.Context, bucketName, objectName, filePath string, opts PutObjectOptions) (n int64, err error) {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := s3utils.CheckValidObjectName(objectName); err != nil {
		return 0, err
	}

	// Open the referenced file.
	fileReader, err := os.Open(filePath)
	// If any error fail quickly here.
	if err != nil {
		return 0, err
	}
	defer fileReader.Close()

	// Save the file stat.
	fileStat, err := fileReader.Stat()
	if err != nil {
		return 0, err
	}

	// Save the file size.
	fileSize := fileStat.Size()

	// Set contentType based on filepath extension if not given or default
	// value of "application/octet-stream" if the extension has no associated type.
	if opts.ContentType == "" {
		if opts.ContentType = mime.TypeByExtension(filepath.Ext(filePath)); opts.ContentType == "" {
			opts.ContentType = "application/octet-stream"
		}
	}
	return c.PutObjectWithContext(ctx, bucketName, objectName, fileReader, fileSize, opts)
}
