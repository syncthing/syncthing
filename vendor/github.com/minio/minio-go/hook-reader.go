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

import "io"

// hookReader hooks additional reader in the source stream. It is
// useful for making progress bars. Second reader is appropriately
// notified about the exact number of bytes read from the primary
// source on each Read operation.
type hookReader struct {
	source io.Reader
	hook   io.Reader
}

// Seek implements io.Seeker. Seeks source first, and if necessary
// seeks hook if Seek method is appropriately found.
func (hr *hookReader) Seek(offset int64, whence int) (n int64, err error) {
	// Verify for source has embedded Seeker, use it.
	sourceSeeker, ok := hr.source.(io.Seeker)
	if ok {
		return sourceSeeker.Seek(offset, whence)
	}
	// Verify if hook has embedded Seeker, use it.
	hookSeeker, ok := hr.hook.(io.Seeker)
	if ok {
		return hookSeeker.Seek(offset, whence)
	}
	return n, nil
}

// Read implements io.Reader. Always reads from the source, the return
// value 'n' number of bytes are reported through the hook. Returns
// error for all non io.EOF conditions.
func (hr *hookReader) Read(b []byte) (n int, err error) {
	n, err = hr.source.Read(b)
	if err != nil && err != io.EOF {
		return n, err
	}
	// Progress the hook with the total read bytes from the source.
	if _, herr := hr.hook.Read(b[:n]); herr != nil {
		if herr != io.EOF {
			return n, herr
		}
	}
	return n, err
}

// newHook returns a io.ReadSeeker which implements hookReader that
// reports the data read from the source to the hook.
func newHook(source, hook io.Reader) io.Reader {
	if hook == nil {
		return source
	}
	return &hookReader{source, hook}
}
