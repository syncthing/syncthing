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
	"io"
)

// PutObjectWithContext - Identical to PutObject call, but accepts context to facilitate request cancellation.
func (c Client) PutObjectWithContext(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64,
	opts PutObjectOptions) (n int64, err error) {
	err = opts.validate()
	if err != nil {
		return 0, err
	}
	if opts.EncryptMaterials != nil {
		if err = opts.EncryptMaterials.SetupEncryptMode(reader); err != nil {
			return 0, err
		}
		return c.putObjectMultipartStreamNoLength(ctx, bucketName, objectName, opts.EncryptMaterials, opts)
	}
	return c.putObjectCommon(ctx, bucketName, objectName, reader, objectSize, opts)
}
