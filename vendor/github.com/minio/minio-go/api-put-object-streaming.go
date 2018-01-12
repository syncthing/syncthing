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
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/minio/minio-go/pkg/s3utils"
)

// putObjectMultipartStream - upload a large object using
// multipart upload and streaming signature for signing payload.
// Comprehensive put object operation involving multipart uploads.
//
// Following code handles these types of readers.
//
//  - *minio.Object
//  - Any reader which has a method 'ReadAt()'
//
func (c Client) putObjectMultipartStream(ctx context.Context, bucketName, objectName string,
	reader io.Reader, size int64, opts PutObjectOptions) (n int64, err error) {

	if !isObject(reader) && isReadAt(reader) {
		// Verify if the reader implements ReadAt and it is not a *minio.Object then we will use parallel uploader.
		n, err = c.putObjectMultipartStreamFromReadAt(ctx, bucketName, objectName, reader.(io.ReaderAt), size, opts)
	} else {
		n, err = c.putObjectMultipartStreamNoChecksum(ctx, bucketName, objectName, reader, size, opts)
	}
	if err != nil {
		errResp := ToErrorResponse(err)
		// Verify if multipart functionality is not available, if not
		// fall back to single PutObject operation.
		if errResp.Code == "AccessDenied" && strings.Contains(errResp.Message, "Access Denied") {
			// Verify if size of reader is greater than '5GiB'.
			if size > maxSinglePutObjectSize {
				return 0, ErrEntityTooLarge(size, maxSinglePutObjectSize, bucketName, objectName)
			}
			// Fall back to uploading as single PutObject operation.
			return c.putObjectNoChecksum(ctx, bucketName, objectName, reader, size, opts)
		}
	}
	return n, err
}

// uploadedPartRes - the response received from a part upload.
type uploadedPartRes struct {
	Error   error // Any error encountered while uploading the part.
	PartNum int   // Number of the part uploaded.
	Size    int64 // Size of the part uploaded.
	Part    *ObjectPart
}

type uploadPartReq struct {
	PartNum int         // Number of the part uploaded.
	Part    *ObjectPart // Size of the part uploaded.
}

// putObjectMultipartFromReadAt - Uploads files bigger than 64MiB.
// Supports all readers which implements io.ReaderAt interface
// (ReadAt method).
//
// NOTE: This function is meant to be used for all readers which
// implement io.ReaderAt which allows us for resuming multipart
// uploads but reading at an offset, which would avoid re-read the
// data which was already uploaded. Internally this function uses
// temporary files for staging all the data, these temporary files are
// cleaned automatically when the caller i.e http client closes the
// stream after uploading all the contents successfully.
func (c Client) putObjectMultipartStreamFromReadAt(ctx context.Context, bucketName, objectName string,
	reader io.ReaderAt, size int64, opts PutObjectOptions) (n int64, err error) {
	// Input validation.
	if err = s3utils.CheckValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err = s3utils.CheckValidObjectName(objectName); err != nil {
		return 0, err
	}

	// Calculate the optimal parts info for a given size.
	totalPartsCount, partSize, lastPartSize, err := optimalPartInfo(size)
	if err != nil {
		return 0, err
	}

	// Initiate a new multipart upload.
	uploadID, err := c.newUploadID(ctx, bucketName, objectName, opts)
	if err != nil {
		return 0, err
	}

	// Aborts the multipart upload in progress, if the
	// function returns any error, since we do not resume
	// we should purge the parts which have been uploaded
	// to relinquish storage space.
	defer func() {
		if err != nil {
			c.abortMultipartUpload(ctx, bucketName, objectName, uploadID)
		}
	}()

	// Total data read and written to server. should be equal to 'size' at the end of the call.
	var totalUploadedSize int64

	// Complete multipart upload.
	var complMultipartUpload completeMultipartUpload

	// Declare a channel that sends the next part number to be uploaded.
	// Buffered to 10000 because thats the maximum number of parts allowed
	// by S3.
	uploadPartsCh := make(chan uploadPartReq, 10000)

	// Declare a channel that sends back the response of a part upload.
	// Buffered to 10000 because thats the maximum number of parts allowed
	// by S3.
	uploadedPartsCh := make(chan uploadedPartRes, 10000)

	// Used for readability, lastPartNumber is always totalPartsCount.
	lastPartNumber := totalPartsCount

	// Send each part number to the channel to be processed.
	for p := 1; p <= totalPartsCount; p++ {
		uploadPartsCh <- uploadPartReq{PartNum: p, Part: nil}
	}
	close(uploadPartsCh)
	// Receive each part number from the channel allowing three parallel uploads.
	for w := 1; w <= opts.getNumThreads(); w++ {
		go func(partSize int64) {
			// Each worker will draw from the part channel and upload in parallel.
			for uploadReq := range uploadPartsCh {

				// If partNumber was not uploaded we calculate the missing
				// part offset and size. For all other part numbers we
				// calculate offset based on multiples of partSize.
				readOffset := int64(uploadReq.PartNum-1) * partSize

				// As a special case if partNumber is lastPartNumber, we
				// calculate the offset based on the last part size.
				if uploadReq.PartNum == lastPartNumber {
					readOffset = (size - lastPartSize)
					partSize = lastPartSize
				}

				// Get a section reader on a particular offset.
				sectionReader := newHook(io.NewSectionReader(reader, readOffset, partSize), opts.Progress)

				// Proceed to upload the part.
				var objPart ObjectPart
				objPart, err = c.uploadPart(ctx, bucketName, objectName, uploadID,
					sectionReader, uploadReq.PartNum,
					"", "", partSize, opts.UserMetadata)
				if err != nil {
					uploadedPartsCh <- uploadedPartRes{
						Size:  0,
						Error: err,
					}
					// Exit the goroutine.
					return
				}

				// Save successfully uploaded part metadata.
				uploadReq.Part = &objPart

				// Send successful part info through the channel.
				uploadedPartsCh <- uploadedPartRes{
					Size:    objPart.Size,
					PartNum: uploadReq.PartNum,
					Part:    uploadReq.Part,
					Error:   nil,
				}
			}
		}(partSize)
	}

	// Gather the responses as they occur and update any
	// progress bar.
	for u := 1; u <= totalPartsCount; u++ {
		uploadRes := <-uploadedPartsCh
		if uploadRes.Error != nil {
			return totalUploadedSize, uploadRes.Error
		}
		// Retrieve each uploaded part and store it to be completed.
		// part, ok := partsInfo[uploadRes.PartNum]
		part := uploadRes.Part
		if part == nil {
			return 0, ErrInvalidArgument(fmt.Sprintf("Missing part number %d", uploadRes.PartNum))
		}
		// Update the totalUploadedSize.
		totalUploadedSize += uploadRes.Size
		// Store the parts to be completed in order.
		complMultipartUpload.Parts = append(complMultipartUpload.Parts, CompletePart{
			ETag:       part.ETag,
			PartNumber: part.PartNumber,
		})
	}

	// Verify if we uploaded all the data.
	if totalUploadedSize != size {
		return totalUploadedSize, ErrUnexpectedEOF(totalUploadedSize, size, bucketName, objectName)
	}

	// Sort all completed parts.
	sort.Sort(completedParts(complMultipartUpload.Parts))
	_, err = c.completeMultipartUpload(ctx, bucketName, objectName, uploadID, complMultipartUpload)
	if err != nil {
		return totalUploadedSize, err
	}

	// Return final size.
	return totalUploadedSize, nil
}

func (c Client) putObjectMultipartStreamNoChecksum(ctx context.Context, bucketName, objectName string,
	reader io.Reader, size int64, opts PutObjectOptions) (n int64, err error) {
	// Input validation.
	if err = s3utils.CheckValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err = s3utils.CheckValidObjectName(objectName); err != nil {
		return 0, err
	}

	// Calculate the optimal parts info for a given size.
	totalPartsCount, partSize, lastPartSize, err := optimalPartInfo(size)
	if err != nil {
		return 0, err
	}
	// Initiates a new multipart request
	uploadID, err := c.newUploadID(ctx, bucketName, objectName, opts)
	if err != nil {
		return 0, err
	}

	// Aborts the multipart upload if the function returns
	// any error, since we do not resume we should purge
	// the parts which have been uploaded to relinquish
	// storage space.
	defer func() {
		if err != nil {
			c.abortMultipartUpload(ctx, bucketName, objectName, uploadID)
		}
	}()

	// Total data read and written to server. should be equal to 'size' at the end of the call.
	var totalUploadedSize int64

	// Initialize parts uploaded map.
	partsInfo := make(map[int]ObjectPart)

	// Part number always starts with '1'.
	var partNumber int
	for partNumber = 1; partNumber <= totalPartsCount; partNumber++ {
		// Update progress reader appropriately to the latest offset
		// as we read from the source.
		hookReader := newHook(reader, opts.Progress)

		// Proceed to upload the part.
		if partNumber == totalPartsCount {
			partSize = lastPartSize
		}
		var objPart ObjectPart
		objPart, err = c.uploadPart(ctx, bucketName, objectName, uploadID,
			io.LimitReader(hookReader, partSize),
			partNumber, "", "", partSize, opts.UserMetadata)
		if err != nil {
			return totalUploadedSize, err
		}

		// Save successfully uploaded part metadata.
		partsInfo[partNumber] = objPart

		// Save successfully uploaded size.
		totalUploadedSize += partSize
	}

	// Verify if we uploaded all the data.
	if size > 0 {
		if totalUploadedSize != size {
			return totalUploadedSize, ErrUnexpectedEOF(totalUploadedSize, size, bucketName, objectName)
		}
	}

	// Complete multipart upload.
	var complMultipartUpload completeMultipartUpload

	// Loop over total uploaded parts to save them in
	// Parts array before completing the multipart request.
	for i := 1; i < partNumber; i++ {
		part, ok := partsInfo[i]
		if !ok {
			return 0, ErrInvalidArgument(fmt.Sprintf("Missing part number %d", i))
		}
		complMultipartUpload.Parts = append(complMultipartUpload.Parts, CompletePart{
			ETag:       part.ETag,
			PartNumber: part.PartNumber,
		})
	}

	// Sort all completed parts.
	sort.Sort(completedParts(complMultipartUpload.Parts))
	_, err = c.completeMultipartUpload(ctx, bucketName, objectName, uploadID, complMultipartUpload)
	if err != nil {
		return totalUploadedSize, err
	}

	// Return final size.
	return totalUploadedSize, nil
}

// putObjectNoChecksum special function used Google Cloud Storage. This special function
// is used for Google Cloud Storage since Google's multipart API is not S3 compatible.
func (c Client) putObjectNoChecksum(ctx context.Context, bucketName, objectName string, reader io.Reader, size int64, opts PutObjectOptions) (n int64, err error) {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return 0, err
	}
	if err := s3utils.CheckValidObjectName(objectName); err != nil {
		return 0, err
	}

	// Size -1 is only supported on Google Cloud Storage, we error
	// out in all other situations.
	if size < 0 && !s3utils.IsGoogleEndpoint(c.endpointURL) {
		return 0, ErrEntityTooSmall(size, bucketName, objectName)
	}
	if size > 0 {
		if isReadAt(reader) && !isObject(reader) {
			seeker, _ := reader.(io.Seeker)
			offset, err := seeker.Seek(0, io.SeekCurrent)
			if err != nil {
				return 0, ErrInvalidArgument(err.Error())
			}
			reader = io.NewSectionReader(reader.(io.ReaderAt), offset, size)
		}
	}

	// Update progress reader appropriately to the latest offset as we
	// read from the source.
	readSeeker := newHook(reader, opts.Progress)

	// This function does not calculate sha256 and md5sum for payload.
	// Execute put object.
	st, err := c.putObjectDo(ctx, bucketName, objectName, readSeeker, "", "", size, opts)
	if err != nil {
		return 0, err
	}
	if st.Size != size {
		return 0, ErrUnexpectedEOF(st.Size, size, bucketName, objectName)
	}
	return size, nil
}

// putObjectDo - executes the put object http operation.
// NOTE: You must have WRITE permissions on a bucket to add an object to it.
func (c Client) putObjectDo(ctx context.Context, bucketName, objectName string, reader io.Reader, md5Base64, sha256Hex string, size int64, opts PutObjectOptions) (ObjectInfo, error) {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return ObjectInfo{}, err
	}
	if err := s3utils.CheckValidObjectName(objectName); err != nil {
		return ObjectInfo{}, err
	}
	// Set headers.
	customHeader := opts.Header()

	// Populate request metadata.
	reqMetadata := requestMetadata{
		bucketName:       bucketName,
		objectName:       objectName,
		customHeader:     customHeader,
		contentBody:      reader,
		contentLength:    size,
		contentMD5Base64: md5Base64,
		contentSHA256Hex: sha256Hex,
	}

	// Execute PUT an objectName.
	resp, err := c.executeMethod(ctx, "PUT", reqMetadata)
	defer closeResponse(resp)
	if err != nil {
		return ObjectInfo{}, err
	}
	if resp != nil {
		if resp.StatusCode != http.StatusOK {
			return ObjectInfo{}, httpRespToErrorResponse(resp, bucketName, objectName)
		}
	}

	var objInfo ObjectInfo
	// Trim off the odd double quotes from ETag in the beginning and end.
	objInfo.ETag = strings.TrimPrefix(resp.Header.Get("ETag"), "\"")
	objInfo.ETag = strings.TrimSuffix(objInfo.ETag, "\"")
	// A success here means data was written to server successfully.
	objInfo.Size = size

	// Return here.
	return objInfo, nil
}
