// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"io"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type s3Copier struct {
	endpoint    string
	region      string
	bucket      string
	key         string
	accessKeyID string
	secretKey   string
}

func newS3Copier(endpoint, region, bucket, key, accessKeyID, secretKey string) *s3Copier {
	return &s3Copier{
		endpoint:    endpoint,
		region:      region,
		bucket:      bucket,
		key:         key,
		accessKeyID: accessKeyID,
		secretKey:   secretKey,
	}
}

func (s *s3Copier) upload(r io.Reader) error {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(s.region),
		Endpoint:    aws.String(s.endpoint),
		Credentials: credentials.NewStaticCredentials(s.accessKeyID, s.secretKey, ""),
	})
	if err != nil {
		return err
	}

	uploader := s3manager.NewUploader(sess)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key),
		Body:   r,
	})
	return err
}

func (s *s3Copier) downloadLatest(w io.WriterAt) error {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(s.region),
		Endpoint:    aws.String(s.endpoint),
		Credentials: credentials.NewStaticCredentials(s.accessKeyID, s.secretKey, ""),
	})
	if err != nil {
		return err
	}

	svc := s3.New(sess)
	resp, err := svc.ListObjectsV2(&s3.ListObjectsV2Input{Bucket: aws.String(s.bucket)})
	if err != nil {
		return err
	}

	var lastKey string
	var lastModified time.Time
	var lastSize int64
	for _, item := range resp.Contents {
		if item.LastModified.After(lastModified) && *item.Size > lastSize {
			lastKey = *item.Key
			lastModified = *item.LastModified
			lastSize = *item.Size
		} else if lastModified.Sub(*item.LastModified) < 5*time.Minute && *item.Size > lastSize {
			lastKey = *item.Key
			lastSize = *item.Size
		}
	}

	log.Println("Downloading database from", lastKey)
	downloader := s3manager.NewDownloader(sess)
	_, err = downloader.Download(w, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(lastKey),
	})
	return err
}
